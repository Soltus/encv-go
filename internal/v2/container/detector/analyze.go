package detector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"log/slog"
	"path/filepath"
	"text/tabwriter"
	"text/template"

	"github.com/Soltus/encv-go/internal/logger"
	"github.com/Soltus/encv-go/internal/v2/chunker"
	"github.com/Soltus/encv-go/internal/v2/container/block"
	containerhandle "github.com/Soltus/encv-go/internal/v2/container/handle"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/fxamacker/cbor/v2"
)

var detectorLogger = logger.WithComponent("detector")

func AnalyzeContainerV2(ctx context.Context, containerPath string, printToStdout bool) (string, error) {
	absPath, err := filepath.Abs(containerPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	src, err := containerhandle.NewFileSource(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to open file '%s': %w", absPath, err)
	}

	h, err := containerhandle.Open(src)
	if err != nil {
		src.Close()
		return "", fmt.Errorf("failed to open container: %w", err)
	}
	defer h.Close()

	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	defer w.Flush()

	fmt.Fprintf(&buf, "--- ENCV Container Analysis: %s (Size: %d bytes) ---\n\n", filepath.Base(absPath), h.Source().Size())

	writeHeaderAnalysis(&buf, w, h)
	w.Flush()

	writeFooterAnalysis(&buf, w, h)
	w.Flush()

	writeManifestAnalysis(&buf, h)

	scanHTML, scannedBlocks, scanErr := performPhysicalLayoutScan(h.Source(), *h.Manifest(), h.Source().Size(), h.HeaderSize())
	if scanErr != nil {
		return "", fmt.Errorf("physical layout scan failed: %w", scanErr)
	}
	buf.WriteString(scanHTML)

	if h.Manifest() != nil {
		var validationHTML string
		var validationErr error
		if h.Version() == 4 {
			validationHTML, validationErr = performCrossValidationV4(h, scannedBlocks)
		} else {
			validationHTML, validationErr = performCrossValidation(h.FooterV2(), scannedBlocks, *h.Manifest())
		}
		if validationErr != nil {
			detectorLogger.Warn("cross-validation failed",
				slog.Any("error", validationErr),
			)
			buf.WriteString(">>> 4. Cross-Validation Report\n  [ERROR] Cross-validation encountered an error. See server logs for details.\n")
		} else {
			buf.WriteString(validationHTML)
		}
	} else {
		buf.WriteString(">>> 4. Cross-Validation Report\n  [INFO] Manifest not available, skipping cross-validation.\n")
	}

	w.Flush()

	content := buf.String()

	if printToStdout {
		fmt.Print(content)
	}

	safeContent := template.HTMLEscapeString(content)

	finalHTML := fmt.Sprintf("<pre><code>%s</code></pre>", safeContent)

	return finalHTML, nil
}

func writeHeaderAnalysis(buf *bytes.Buffer, w *tabwriter.Writer, h containerhandle.ContainerHandle) {
	fmt.Fprintln(buf, ">>> 0. Header Analysis (from beginning of file)")

	switch h.Version() {
	case 4:
		hdr := h.HeaderV4()
		fmt.Fprintf(w, "  Version:\t%d (V4)\n", hdr.Version)
		fmt.Fprintf(w, "  Magic:\t%s\n", string(hdr.Magic[:]))
		flagStr := ""
		if hdr.Flags&types.FlagIsMainContainer != 0 {
			flagStr += "MainContainer "
		}
		if hdr.Flags&types.FlagIsPhysicalChunk != 0 {
			flagStr += "PhysicalChunk "
		}
		fmt.Fprintf(w, "  Flags:\t0x%04x (%s)\n", hdr.Flags, flagStr)
		fmt.Fprintf(w, "  ID Type:\t%d\n", hdr.IDType)
		fmt.Fprintf(w, "  ID Length:\t%d bytes\n", hdr.IDLength)
		fmt.Fprintf(w, "  Header CRC32:\t%08x (Verified OK)\n", hdr.HeaderCRC32)
		fmt.Fprintf(w, "  ContainerType:\t%d\n", hdr.ContainerType)
		fmt.Fprintf(w, "  IsSeekable:\t%v\n", hdr.IsSeekable)
		fmt.Fprintf(w, "  ManifestOffset:\t%d\n", hdr.ManifestOffset)
		fmt.Fprintf(w, "  ManifestLength:\t%d\n", hdr.ManifestLength)
		w.Flush()

	case 3:
		hdr := h.HeaderV3()
		fmt.Fprintf(w, "  Version:\t%d (V3)\n", hdr.Version)
		fmt.Fprintf(w, "  Magic:\t%s\n", string(hdr.Magic[:]))
		flagStr := ""
		if hdr.Flags&types.FlagIsMainContainer != 0 {
			flagStr += "MainContainer "
		}
		if hdr.Flags&types.FlagIsPhysicalChunk != 0 {
			flagStr += "PhysicalChunk "
		}
		fmt.Fprintf(w, "  Flags:\t0x%04x (%s)\n", hdr.Flags, flagStr)
		fmt.Fprintf(w, "  ID Type:\t%d\n", hdr.IDType)
		fmt.Fprintf(w, "  ID Length:\t%d bytes\n", hdr.IDLength)
		fmt.Fprintf(w, "  Header CRC32:\t%08x (Verified OK)\n", hdr.HeaderCRC32)

		if hdr.IDType == uint32(types.IDType_CBOR) && hdr.IDLength > 0 {
			specialIDBytes := hdr.SpecialID[:hdr.IDLength]
			var meta map[string]interface{}
			if err := cbor.Unmarshal(specialIDBytes, &meta); err == nil {
				fmt.Fprintln(buf, "  SpecialID Content (CBOR):")
				for k, v := range meta {
					fmt.Fprintf(w, "    %s:\t%v\n", k, v)
				}
			} else {
				fmt.Fprintf(buf, "  SpecialID Content:\t[CBOR Parse Failed: %v]\n", err)
			}
		}
		w.Flush()

	case 2:
		fmt.Fprintf(buf, "  Version:\t2 (V2)\n")
	}

	fmt.Fprintln(buf)
}

func writeFooterAnalysis(buf *bytes.Buffer, w *tabwriter.Writer, h containerhandle.ContainerHandle) {
	fmt.Fprintln(buf, ">>> 1. Footer Analysis (from end of file)")

	if h.Version() == 4 {
		ftr := h.FooterV4()
		if ftr != nil {
			fmt.Fprintf(w, "  Status:\tOK\n")
			fmt.Fprintf(w, "  Magic:\t%s\n", string(ftr.Magic[:]))
			fmt.Fprintf(w, "  Global CRC32:\t%08x\n", ftr.GlobalCRC32)
			w.Flush()
			fmt.Fprintln(buf)
		} else {
			fmt.Fprintf(buf, "  Status: FAILED\n  Reason: v4 footer not available\n\n")
		}
	} else {
		ftr := h.FooterV2()
		if ftr != nil {
			fmt.Fprintf(w, "  Status:\tOK\n")
			fmt.Fprintf(w, "  Magic:\t%s\n", string(ftr.Magic[:]))
			fmt.Fprintf(w, "  Manifest Offset:\t%d\n", ftr.ManifestOffset)
			fmt.Fprintf(w, "  Manifest Length:\t%d\n", ftr.ManifestLength)
			fmt.Fprintf(w, "  Manifest CRC32:\t%08x\n", ftr.ManifestCRC32)
			fmt.Fprintf(w, "  Global CRC32:\t%08x\n", ftr.GlobalCRC32)
			w.Flush()
			fmt.Fprintln(buf)
		} else {
			fmt.Fprintf(buf, "  Status: FAILED\n  Reason: v2/v3 footer not available\n\n")
		}
	}
}

func writeManifestAnalysis(buf *bytes.Buffer, h containerhandle.ContainerHandle) {
	fmt.Fprintln(buf, ">>> 2. Manifest Analysis")

	manifestObj := h.Manifest()
	if manifestObj == nil {
		fmt.Fprintln(buf, "  Status: NOT AVAILABLE")
		fmt.Fprintln(buf)
		return
	}

	fmt.Fprintf(buf, "  Source:\tContainerHandle\n")
	rawJSON, _ := json.Marshal(manifestObj)
	fmt.Fprintf(buf, "  Raw Size:\t%d bytes\n\n", len(rawJSON))

	fmt.Fprintln(buf, "  Content (pretty-printed):")
	prettyJSON, _ := json.MarshalIndent(manifestObj, "    ", "  ")
	fmt.Fprintf(buf, "    %s\n", string(prettyJSON))
	fmt.Fprintln(buf)
}

type scannedBlock struct {
	offset int64
	header *block.BlockHeader_v2
	crc    uint32
}

func performPhysicalLayoutScan(source io.ReaderAt, manifestObj types.Manifest, fileSize int64, headerSize int64) (string, []scannedBlock, error) {
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	defer w.Flush()

	fmt.Fprintln(&buf, ">>> 3. Physical Layout Scan (Block-by-Block)")

	isChunked := manifestObj.Version != 0 && !chunker.IsSingleFileContainer(&manifestObj)
	if isChunked {
		fmt.Fprintln(&buf, "  [INFO] Detected a physical chunked container. Analyzing main file layout.")
	} else {
		fmt.Fprintln(&buf, "  [INFO] Detected a single-file container. Scanning for all blocks.")
	}

	fmt.Fprintln(w, "  Offset\t\tType\t\tLength\tCRC32")
	fmt.Fprintln(w, "  ------\t\t----\t\t------\t------")

	var scannedBlocks []scannedBlock

	if isChunked {
		var firstChunkLength uint64
		for _, frag := range manifestObj.Fragments {
			if frag.ID == "logical_fragment_0" {
				firstChunkLength = frag.Length
				break
			}
		}
		if firstChunkLength == 0 {
			return "", nil, fmt.Errorf("could not find length for logical_fragment_0 in manifest")
		}

		crc, err := streamCRC32(source, headerSize, firstChunkLength)
		if err != nil {
			return "", nil, fmt.Errorf("failed to stream CRC for first chunk: %w", err)
		}

		scannedBlocks = append(scannedBlocks, scannedBlock{
			offset: headerSize,
			header: &block.BlockHeader_v2{Type: types.BlockTypeData_v2, Length: firstChunkLength},
			crc:    crc,
		})
		fmt.Fprintf(w, "  %d\t\t%s\t\t%d\t%08x (raw stream)\n", headerSize, types.GetBlockTypeName(uint32(types.BlockTypeData_v2)), firstChunkLength, crc)

		fmt.Fprintln(&buf, "  [INFO] Re-scanning for manifest block...")
		manifestOffset, manifestBytes, err := scanForManifestWithOffset(source, headerSize, fileSize)
		if err != nil {
			return "", nil, fmt.Errorf("failed to locate manifest in main file: %w", err)
		}
		manifestScannedBlock := scannedBlock{
			offset: manifestOffset,
			header: &block.BlockHeader_v2{Type: types.BlockTypeManifest_v2, Length: uint64(len(manifestBytes))},
			crc:    crc32.ChecksumIEEE(manifestBytes),
		}
		scannedBlocks = append(scannedBlocks, manifestScannedBlock)
		fmt.Fprintf(w, "  %d\t\t%s\t\t%d\t%08x\n", manifestScannedBlock.offset, types.GetBlockTypeName(uint32(manifestScannedBlock.header.Type)), manifestScannedBlock.header.Length, manifestScannedBlock.crc)

	} else {
		fmt.Fprintln(&buf, "  [INFO] Scanning for all blocks.")

		sectionReader := io.NewSectionReader(source, headerSize, fileSize-headerSize)

		currentOffset := headerSize
		for {
			if currentOffset >= fileSize {
				break
			}

			header, err := block.ReadBlockHeader(sectionReader)
			if err != nil {
				if err == io.EOF {
					break
				}
				return "", nil, fmt.Errorf("failed to read block header at offset %d: %w", currentOffset, err)
			}

			hasher := crc32.NewIEEE()
			dataBuf := make([]byte, 32*1024)
			var remaining uint64 = header.Length
			for remaining > 0 {
				readSize := int64(len(dataBuf))
				if remaining < uint64(len(dataBuf)) {
					readSize = int64(remaining)
				}
				n, err := sectionReader.Read(dataBuf[:readSize])
				if err != nil && err != io.EOF {
					return "", nil, fmt.Errorf("failed to read block data: %w", err)
				}
				if n == 0 {
					break
				}
				if _, err := hasher.Write(dataBuf[:n]); err != nil {
					return "", nil, fmt.Errorf("failed to write to hasher: %w", err)
				}
				remaining -= uint64(n)
			}
			crc := hasher.Sum32()
			scannedBlocks = append(scannedBlocks, scannedBlock{offset: currentOffset, header: header, crc: crc})
			fmt.Fprintf(w, "  %d\t\t%s\t\t%d\t%08x\n", currentOffset, types.GetBlockTypeName(uint32(header.Type)), header.Length, crc)
			currentOffset += int64(header.Length) + block.GetBlockHeader_v2_Size()
		}
	}

	w.Flush()
	return buf.String(), scannedBlocks, nil
}

func performCrossValidation(footer *types.EnvelopeFooter_v2, scannedBlocks []scannedBlock, manifestObj types.Manifest) (string, error) {
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	defer w.Flush()

	fmt.Fprintln(&buf, ">>> 4. Cross-Validation Report")

	if footer != nil {
		manifestBlock := findBlockByType(scannedBlocks, uint32(types.BlockTypeManifest_v2))
		if manifestBlock != nil {
			if uint64(manifestBlock.offset) == footer.ManifestOffset {
				fmt.Fprintf(&buf, "  [OK] Footer ManifestOffset (%d) matches scanned block offset (%d).\n", footer.ManifestOffset, manifestBlock.offset)
			} else {
				fmt.Fprintf(&buf, "  [ERROR] Mismatch! Footer ManifestOffset is %d, but scanned Manifest block is at %d.\n", footer.ManifestOffset, manifestBlock.offset)
			}
			if uint32(manifestBlock.crc) == footer.ManifestCRC32 {
				fmt.Fprintf(&buf, "  [OK] Footer ManifestCRC32 (%08x) matches scanned block CRC32 (%08x).\n", footer.ManifestCRC32, manifestBlock.crc)
			} else {
				fmt.Fprintf(&buf, "  [ERROR] Mismatch! Footer ManifestCRC32 is %08x, but scanned block CRC32 is %08x.\n", footer.ManifestCRC32, manifestBlock.crc)
			}
		} else {
			fmt.Fprintf(&buf, "  [ERROR] Footer is valid, but no Manifest block was found during scan.\n")
		}
	} else {
		fmt.Fprintln(&buf, "  [INFO] Footer is invalid, skipping Footer-related validation.")
	}

	var dataBlockFrags int
	for _, frag := range manifestObj.Fragments {
		if frag.Type == types.FragmentType_SeekableStream || frag.Type == types.FragmentType_AtomicFile {
			dataBlockFrags++
		}
	}
	dataBlocks := findAllBlocksByType(scannedBlocks, uint32(types.BlockTypeData_v2))
	if len(dataBlocks) == dataBlockFrags {
		fmt.Fprintf(&buf, "  [OK] Scanned Data Blocks (%d) count matches Manifest Data Fragments (%d).\n", len(dataBlocks), dataBlockFrags)
	} else {
		fmt.Fprintf(&buf, "  [ERROR] Mismatch! Found %d Data Blocks in file, but Manifest lists %d Data Fragments (SeekableStream + AtomicFile).\n", len(dataBlocks), dataBlockFrags)
	}

	fmt.Fprintln(&buf, "\n  --- Fragment Offset Mapping ---")
	fmt.Fprintln(w, "    Fragment ID\t\tType\t\tGlobal Start Offset\tPhysical Block Offset")
	fmt.Fprintln(w, "    -----------\t\t----\t\t------------------\t-------------------")
	fragIndex := 0
	for _, frag := range manifestObj.Fragments {
		if frag.Type == types.FragmentType_SeekableStream || frag.Type == types.FragmentType_AtomicFile {
			physicalOffset := "<not found>"
			if fragIndex < len(dataBlocks) {
				physicalOffset = fmt.Sprintf("%d", dataBlocks[fragIndex].offset)
			}
			fmt.Fprintf(w, "    %s\t\t%s\t\t%d\t\t%s\n", frag.ID, frag.Type, frag.GlobalStartOffset, physicalOffset)
			fragIndex++
		}
	}

	w.Flush()
	return buf.String(), nil
}

func performCrossValidationV4(h containerhandle.ContainerHandle, scannedBlocks []scannedBlock) (string, error) {
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	defer w.Flush()

	fmt.Fprintln(&buf, ">>> 4. Cross-Validation Report (V4)")

	v4Header := h.HeaderV4()
	v4Footer := h.FooterV4()
	if v4Header == nil || v4Footer == nil {
		return "", fmt.Errorf("v4 header or footer not available")
	}

	source := h.Source()

	headerCRC, err := streamCRC32(source, 0, uint64(h.HeaderSize()))
	if err != nil {
		return "", fmt.Errorf("failed to compute header CRC: %w", err)
	}
	if headerCRC == v4Header.HeaderCRC32 {
		fmt.Fprintf(&buf, "  [OK] Header CRC32 matches computed value (%08x).\n", headerCRC)
	} else {
		fmt.Fprintf(&buf, "  [ERROR] Header CRC32 mismatch! Stored: %08x, Computed: %08x.\n", v4Header.HeaderCRC32, headerCRC)
	}

	dataBlocks := findAllBlocksByType(scannedBlocks, uint32(types.BlockTypeData_v2))
	totalDataCRC := crc32.NewIEEE()
	for _, b := range dataBlocks {
		totalDataCRC.Write([]byte{byte(b.crc), byte(b.crc >> 8), byte(b.crc >> 16), byte(b.crc >> 24)})
	}

	fmt.Fprintf(&buf, "  [INFO] V4 Footer GlobalCRC32: %08x\n", v4Footer.GlobalCRC32)
	fmt.Fprintf(&buf, "  [INFO] Scanned %d data blocks, manifest stored in V4 Header (offset=%d, length=%d).\n",
		len(dataBlocks), v4Header.ManifestOffset, v4Header.ManifestLength)

	w.Flush()
	return buf.String(), nil
}

func scanForManifestWithOffset(source io.ReaderAt, headerSize int64, fileSize int64) (int64, []byte, error) {
	remaining := fileSize - headerSize
	if remaining <= 0 {
		return 0, nil, fmt.Errorf("no data after header")
	}

	sectionReader := io.NewSectionReader(source, headerSize, remaining)
	currentOffset := headerSize

	for {
		if currentOffset >= fileSize {
			break
		}

		header, err := block.ReadBlockHeader(sectionReader)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to read block header: %w", err)
		}
		if header.Type == types.BlockTypeManifest_v2 {
			data, err := block.ReadBlockData(sectionReader, header)
			if err != nil {
				return 0, nil, fmt.Errorf("failed to read manifest data: %w", err)
			}
			return currentOffset, data, nil
		}
		skipBytes := int64(header.Length)
		if skipBytes > 0 {
			if _, err := sectionReader.Seek(skipBytes, io.SeekCurrent); err != nil {
				return 0, nil, fmt.Errorf("failed to skip block data: %w", err)
			}
		}
		currentOffset += block.GetBlockHeader_v2_Size() + skipBytes
	}

	return 0, nil, fmt.Errorf("manifest block not found")
}

func findBlockByType(blocks []scannedBlock, blockType uint32) *scannedBlock {
	for _, b := range blocks {
		if b.header.Type == uint16(blockType) {
			return &b
		}
	}
	return nil
}

func findAllBlocksByType(blocks []scannedBlock, blockType uint32) []scannedBlock {
	var result []scannedBlock
	for _, b := range blocks {
		if b.header.Type == uint16(blockType) {
			result = append(result, b)
		}
	}
	return result
}

func streamCRC32(source io.ReaderAt, startOffset int64, length uint64) (uint32, error) {
	hasher := crc32.NewIEEE()
	buf := make([]byte, 32*1024)

	var offset int64 = startOffset
	var remaining uint64 = length
	for remaining > 0 {
		readSize := int64(len(buf))
		if remaining < uint64(len(buf)) {
			readSize = int64(remaining)
		}

		n, err := source.ReadAt(buf[:readSize], offset)
		if err != nil && err != io.EOF {
			return 0, fmt.Errorf("failed to read from source for CRC stream: %w", err)
		}
		if n == 0 {
			break
		}

		if _, err := hasher.Write(buf[:n]); err != nil {
			return 0, fmt.Errorf("failed to write to hasher: %w", err)
		}
		offset += int64(n)
		remaining -= uint64(n)
	}

	return hasher.Sum32(), nil
}
