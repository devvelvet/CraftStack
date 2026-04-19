package master

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "craftstack/gen/proto/craftstack"
	"craftstack/internal/master/store"
	msync "craftstack/internal/master/sync"
)

const chunkSize = 256 * 1024 // 256KB

// FilePusher pushes files to agents via gRPC SyncService.PushFile.
type FilePusher struct {
	srv *GRPCServer
	log *slog.Logger
}

// NewFilePusher creates a new file pusher.
func NewFilePusher(srv *GRPCServer, log *slog.Logger) *FilePusher {
	return &FilePusher{srv: srv, log: log}
}

// PushToAgents pushes a file to all target agents.
// This is the SyncCallback implementation.
func (p *FilePusher) PushToAgents(file msync.FileInfo, agents []msync.TargetAgent) error {
	if file.AbsPath == "" {
		// delete event — current logonly
		p.log.Info("delete file event (before not implemented)",
			"file", file.RelPath,
			"agents", len(agents),
		)
		return nil
	}

	var lastErr error
	for _, agent := range agents {
		if err := p.pushFileToAgent(file, agent); err != nil {
			p.log.Error("file transfer failed",
				"file", file.RelPath,
				"agent", agent.Name,
				"error", err,
			)
			lastErr = err
		}
	}
	return lastErr
}

func (p *FilePusher) pushFileToAgent(file msync.FileInfo, agent msync.TargetAgent) error {
	// agent gRPC server connect
	conn, err := grpc.NewClient(
		agent.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("agent connection failed (%s): %w", agent.Address, err)
	}
	defer conn.Close()

	// file open and hash calculate
	f, err := os.Open(file.AbsPath)
	if err != nil {
		return fmt.Errorf("file open failed: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("file info query failed: %w", err)
	}

	// hash calculate (file.Hash already if present reuse)
	fileHash := file.Hash
	if fileHash == "" {
		hasher := sha256.New()
		if _, err := io.Copy(hasher, f); err != nil {
			return fmt.Errorf("hash calculate failed: %w", err)
		}
		fileHash = hex.EncodeToString(hasher.Sum(nil))
		f.Seek(0, 0)
	}

	totalChunks := int32((stat.Size() + chunkSize - 1) / chunkSize)
	if totalChunks == 0 {
		totalChunks = 1
	}

	syncID := uuid.New().String()

	p.log.Info("file transfer start",
		"file", file.RelPath,
		"size", stat.Size(),
		"chunks", totalChunks,
		"agent", agent.Name,
		"address", agent.Address,
	)

	// gRPC stream open
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client := pb.NewSyncServiceClient(conn)
	stream, err := client.PushFile(ctx)
	if err != nil {
		return fmt.Errorf("PushFile stream create failed: %w", err)
	}

	// chunk send
	buf := make([]byte, chunkSize)
	var chunkIdx int32

	for {
		n, readErr := f.Read(buf)
		if n == 0 && readErr == io.EOF {
			break
		}

		isLast := readErr == io.EOF

		chunk := &pb.FileChunk{
			Data:        buf[:n],
			ChunkIndex:  chunkIdx,
			TotalChunks: totalChunks,
			IsLast:      isLast,
		}

		// first th chunk metadata include
		if chunkIdx == 0 {
			chunk.Metadata = &pb.FileMetadata{
				FilePath:      file.RelPath,
				FileHash:      fileHash,
				FileSize:      stat.Size(),
				TargetAgentId: agent.ID,
				SyncId:        syncID,
			}
		}

		if err := stream.Send(chunk); err != nil {
			return fmt.Errorf("chunk send failed (chunk %d): %w", chunkIdx, err)
		}

		chunkIdx++

		if isLast {
			break
		}
		if readErr != nil && readErr != io.EOF {
			return fmt.Errorf("file read error: %w", readErr)
		}
	}

	// response receive
	resp, err := stream.CloseAndRecv()
	if err != nil {
		return fmt.Errorf("response receive failed: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("agent receive failed: %s", resp.Message)
	}

	p.log.Info("file transfer complete",
		"file", file.RelPath,
		"hash", fileHash[:12],
		"agent", agent.Name,
		"sync_id", syncID,
	)

	// DB sync history record
	agentID := agent.ID
	p.srv.db.CreateSyncRecord(&store.SyncRecord{
		NodeID:   &agentID,
		FilePath: file.RelPath,
		FileSize: stat.Size(),
		FileHash: fileHash,
		Action:   "push",
		Status:   "completed",
	})

	return nil
}
