package server

// mobile_sparse.go — Sparse container：write / probe / cleanup。

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/Soltus/encv-go/internal/v2/testutil"
	"github.com/gin-gonic/gin"
)

func (s *Server) handleSparseContainerWriteGin(c *gin.Context) {
	var req struct {
		FragmentCount   int    `json:"fragmentCount"`
		FragmentSizeGB  int    `json:"fragmentSizeGB"`
		PhysicalChunkMB int    `json:"physicalChunkMB"`
		ContainerType   uint16 `json:"containerType"`
		BaseName        string `json:"baseName"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json: " + err.Error()})
		return
	}

	outputDir := "/tmp/encv-sparse-test"
	if req.BaseName == "" {
		req.BaseName = fmt.Sprintf("sparse-%d", time.Now().Unix())
	}

	// 1GB = 1024^3
	fragmentSize := int64(req.FragmentSizeGB) * 1024 * 1024 * 1024
	if fragmentSize <= 0 {
		fragmentSize = 128 * 1024 * 1024 * 1024
	}

	cfg := testutil.SparseContainerConfig{
		OutputDir:       outputDir,
		BaseName:        req.BaseName,
		FragmentCount:   req.FragmentCount,
		FragmentSize:    fragmentSize,
		PhysicalChunkMB: req.PhysicalChunkMB,
		ContainerType:   req.ContainerType,
		CipherMode:      0,
	}
	if cfg.ContainerType == 0 {
		cfg.ContainerType = 1
	}
	if cfg.FragmentCount == 0 {
		cfg.FragmentCount = 100
	}

	res, err := testutil.WriteSparseVirtualContainer(cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	slog.Info("[sparse-container] wrote",
		"baseName", req.BaseName,
		"virtualGB", res.VirtualTotal/(1024*1024*1024),
		"physicalKB", res.PhysicalUsed/1024,
		"isSparse", res.IsSparse,
		"durationMs", res.DurationMs,
	)
	c.JSON(http.StatusOK, res)
}

func (s *Server) handleSparseContainerProbeGin(c *gin.Context) {
	mainPath := c.Query("mainPath")
	if mainPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mainPath required"})
		return
	}
	fragmentIdx := 0
	if v := c.Query("fragmentIdx"); v != "" {
		fmt.Sscanf(v, "%d", &fragmentIdx)
	}
	fragmentSizeGB := 128
	if v := c.Query("fragmentSizeGB"); v != "" {
		fmt.Sscanf(v, "%d", &fragmentSizeGB)
	}
	fragmentSize := int64(fragmentSizeGB) * 1024 * 1024 * 1024

	probe, err := testutil.ReadSparseContainerEdgeProbe(mainPath, fragmentIdx, fragmentSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	slog.Info("[sparse-container] probe",
		"mainPath", mainPath,
		"fragmentIdx", fragmentIdx,
		"seekMs", probe.SeekDurationMs,
		"readMs", probe.ReadDurationMs,
		"heapInUseKB", probe.HeapInUseKB,
	)
	c.JSON(http.StatusOK, probe)
}

func (s *Server) handleSparseContainerCleanupGin(c *gin.Context) {
	baseName := c.Query("baseName")
	outputDir := "/tmp/encv-sparse-test"

	if baseName == "" {
		// 清理整个目录
		if err := os.RemoveAll(outputDir); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		slog.Info("[sparse-container] cleaned up entire output dir", "dir", outputDir)
	} else {
		if err := testutil.CleanupSparseContainer(outputDir, baseName); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		slog.Info("[sparse-container] cleaned up", "baseName", baseName)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "baseName": baseName, "outputDir": outputDir})
}
