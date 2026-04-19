package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	pb "craftstack/gen/proto/craftstack"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// syncServiceImpl implements pb.SyncServiceServer on the Agent side.
// Handles receiving files from the master via PushFile.
type syncServiceImpl struct {
	pb.UnimplementedSyncServiceServer
	baseDir string // default path for instance files
	log     *slog.Logger

	// OnFileReceived is called after a file is successfully saved.
	OnFileReceived func(filePath, hash string, size int64)
}

// PushFile receives a file streamed from master in chunks.
func (s *syncServiceImpl) PushFile(stream grpc.ClientStreamingServer[pb.FileChunk, pb.PushFileResponse]) error {
	var metadata *pb.FileMetadata
	var tmpFile *os.File
	var tmpPath string
	var hasher = sha256.New()
	var totalReceived int64

	defer func() {
		if tmpFile != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
		}
	}()

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return status.Errorf(codes.Internal, "stream receive error: %v", err)
		}

		// first th chunk from metadata extract
		if chunk.Metadata != nil && metadata == nil {
			metadata = chunk.Metadata
			s.log.Info("file receive start",
				"file", metadata.FilePath,
				"size", metadata.FileSize,
				"sync_id", metadata.SyncId,
			)

			// target directory create
			destDir := filepath.Join(s.baseDir, filepath.Dir(metadata.FilePath))
			if err := os.MkdirAll(destDir, 0755); err != nil {
				return status.Errorf(codes.Internal, "directory create failed: %v", err)
			}

			// temporary create file
			tmpPath = filepath.Join(destDir, ".craftstack_tmp_"+filepath.Base(metadata.FilePath))
			tmpFile, err = os.Create(tmpPath)
			if err != nil {
				return status.Errorf(codes.Internal, "temporary create file failed: %v", err)
			}
		}

		if tmpFile == nil {
			return status.Errorf(codes.FailedPrecondition, "metadata without data receive")
		}

		// data write
		if len(chunk.Data) > 0 {
			n, err := tmpFile.Write(chunk.Data)
			if err != nil {
				return status.Errorf(codes.Internal, "file write error: %v", err)
			}
			hasher.Write(chunk.Data)
			totalReceived += int64(n)
		}

		if chunk.IsLast {
			break
		}
	}

	if metadata == nil {
		return status.Errorf(codes.InvalidArgument, "file metadata received failed")
	}

	// temporary file close
	tmpFile.Close()

	// hash verify
	receivedHash := hex.EncodeToString(hasher.Sum(nil))
	if metadata.FileHash != "" && receivedHash != metadata.FileHash {
		os.Remove(tmpPath)
		return status.Errorf(codes.DataLoss,
			"hash mismatch: expected %s, receive %s", metadata.FileHash, receivedHash)
	}

	// move to final path (atomic rename)
	finalPath := filepath.Join(s.baseDir, metadata.FilePath)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		// rename failed when copy fallback (cross-device)
		if err := copyFile(tmpPath, finalPath); err != nil {
			return status.Errorf(codes.Internal, "file move failed: %v", err)
		}
		os.Remove(tmpPath)
	}
	tmpFile = nil // defer from delete prevent

	s.log.Info("file receive complete",
		"file", metadata.FilePath,
		"size", totalReceived,
		"hash", receivedHash[:12],
	)

	if s.OnFileReceived != nil {
		s.OnFileReceived(finalPath, receivedHash, totalReceived)
	}

	return stream.SendAndClose(&pb.PushFileResponse{
		Success:      true,
		Message:      "receive complete",
		ReceivedHash: receivedHash,
		SyncId:       metadata.SyncId,
	})
}

// PullFile sends a file from agent to master (e.g., for backup retrieval).
func (s *syncServiceImpl) PullFile(req *pb.PullFileRequest, stream grpc.ServerStreamingServer[pb.FileChunk]) error {
	filePath := filepath.Join(s.baseDir, req.FilePath)

	f, err := os.Open(filePath)
	if err != nil {
		return status.Errorf(codes.NotFound, "file open failed: %v", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return status.Errorf(codes.Internal, "file info query failed: %v", err)
	}

	// hash calculate
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return status.Errorf(codes.Internal, "hash calculate failed: %v", err)
	}
	fileHash := hex.EncodeToString(hasher.Sum(nil))
	f.Seek(0, 0)

	const chunkSize = 256 * 1024
	totalChunks := int32((stat.Size() + chunkSize - 1) / chunkSize)
	if totalChunks == 0 {
		totalChunks = 1
	}

	buf := make([]byte, chunkSize)
	var chunkIdx int32

	for {
		n, readErr := f.Read(buf)
		isLast := readErr == io.EOF || (readErr == nil && int64(n) < chunkSize)

		chunk := &pb.FileChunk{
			Data:        buf[:n],
			ChunkIndex:  chunkIdx,
			TotalChunks: totalChunks,
			IsLast:      isLast,
		}

		// first th chunk metadata include
		if chunkIdx == 0 {
			chunk.Metadata = &pb.FileMetadata{
				FilePath:      req.FilePath,
				FileHash:      fileHash,
				FileSize:      stat.Size(),
				TargetAgentId: req.AgentId,
			}
		}

		if err := stream.Send(chunk); err != nil {
			return err
		}

		chunkIdx++

		if isLast || readErr == io.EOF {
			break
		}
		if readErr != nil {
			return status.Errorf(codes.Internal, "file read error: %v", readErr)
		}
	}

	return nil
}

// AckSync acknowledges that a sync operation completed.
func (s *syncServiceImpl) AckSync(ctx context.Context, req *pb.AckSyncRequest) (*pb.AckSyncResponse, error) {
	s.log.Info("sync check",
		"sync_id", req.SyncId,
		"agent_id", req.AgentId,
		"success", req.Success,
	)

	if !req.Success {
		s.log.Warn("sync failed", "sync_id", req.SyncId, "error", req.ErrorMessage)
	}

	return &pb.AckSyncResponse{Acknowledged: true}, nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create dst: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
