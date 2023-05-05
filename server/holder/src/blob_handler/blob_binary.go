// //////////////////////////////
// 2022 SHLab all rights reserved
// //////////////////////////////

package blob_handler

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/common/definition"
	. "github.com/common/zaplog"
	"go.uber.org/zap"
)

// the name must be captial
type ChunkHeader struct {
	BlobId   [definition.F_BLOBID_SIZE]byte   // 128
	Size     int64                            // 8
	Checksum [definition.F_CHECKSUM_SIZE]byte // 32
}
type Chunk struct {
	ChunkHeader ChunkHeader
	Content     [definition.F_CONTENT_SIZE]byte
}
type BinHeader struct {
	// use `new(T)` to initial:
	RWLock *sync.RWMutex

	ShardId   int
	TripletId string

	// Local fs name
	LocalName string
	// URL of position in object storage.
	RemoteName string

	CurOff int64
}

// TODO: use index to wrap indexHeader, 1 index can contain
// multiple indexHeaders, use this way to improve the parallel
// write bandwidth.
// type Index struct {
// 	ShardId    int
// 	headers []IndexHeader
// }

// shardId is the holder instance id.
func (bh *BinHeader) New(shardId int, triId string) int64 {
	bh.RWLock = new(sync.RWMutex)

	bh.ShardId = shardId
	bh.TripletId = triId
	localfsPrefix := definition.BlobLocalPathPrefix
	if localfsPrefix == "" {
		localfsPrefix = "/var/lib/docker/.cache"
	}
	bh.LocalName =
		fmt.Sprintf("%s/binary_%d_%s.dat", localfsPrefix, shardId, triId)
	info, err := os.Stat(bh.LocalName)
	if os.IsNotExist(err) {
		bh.CurOff = 0
		return 0
	} else if err != nil {
		ZapLogger.Fatal("os.Stat", zap.Any("file", bh.LocalName), zap.Any("err", err))
	}
	bh.CurOff = info.Size()
	return info.Size()
}

func (bh *BinHeader) Put(blobId string, binary []byte) (int64, int64) {
	bh.RWLock.Lock()
	defer bh.RWLock.Unlock()
	var encoded []byte
	if definition.F_4K_Align {
		encoded = Encode4K(blobId, binary)
	} else {
		encoded = Encode(blobId, binary)
	}
	offset, sizeWritten := bh.flush(&encoded)
	bh.CurOff += sizeWritten
	ZapLogger.Info("Put blob succeeded", zap.Any("blobId", blobId),
		zap.Any("offset", offset), zap.Any("sizeWritten", sizeWritten))
	return offset, sizeWritten
}

func (bh *BinHeader) Get(blobId string, offset int64) (binary []byte) {
	bh.RWLock.RLock()
	defer bh.RWLock.RUnlock()
	var data []byte
	if definition.F_4K_Align {
		data = bh.readBlob4K(blobId, offset)
	} else {
		data = bh.readBlob(blobId, offset)
	}
	ZapLogger.Info("Get blob succeeded", zap.Any("blobId", blobId),
		zap.Any("offset", offset), zap.Any("size read", len(data)))
	return data
}

func (bh *BinHeader) flush(binary *[]byte) (int64, int64) {
	f, err := os.OpenFile(bh.LocalName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0755)
	if err != nil {
		ZapLogger.Fatal("os.OpenFile", zap.Any("file", bh.LocalName), zap.Any("err", err))
	}
	defer f.Close()
	// Persist
	written := 0
	if written, err = f.Write(*binary); err != nil {
		ZapLogger.Fatal("f.Write", zap.Any("file", bh.LocalName), zap.Any("err", err))
	}
	return bh.CurOff, int64(written)
}

func (bh *BinHeader) readBlob(blbId string, offset int64) (blobBody []byte) {
	f, err := os.OpenFile(bh.LocalName, os.O_RDONLY, 0755)
	if err != nil {
		ZapLogger.Fatal("os.OpenFile", zap.Any("file", bh.LocalName), zap.Any("err", err))
	}
	defer f.Close()

	idAndSize := make([]byte, 136)
	if _, err = f.ReadAt(idAndSize, offset); err != nil {
		ZapLogger.Fatal("f.readAt", zap.Any("file", bh.LocalName), zap.Any("err", err))
	}

	idOnDisk := DecodeName(idAndSize[:128])
	if strings.Compare(blbId, idOnDisk) != 0 {
		ZapLogger.Fatal("blob name mismatch",
			zap.Any("blobId", blbId),
			zap.Any("idOnDisk", idOnDisk))
	}

	cntSize := DecodeSize(idAndSize[128:136])
	bodyBytes := make([]byte, cntSize)
	start := time.Now()
	if _, err = f.ReadAt(bodyBytes, offset+136); err != nil {
		ZapLogger.Fatal("f.Read failed", zap.Any("err", err))
	}
	duration := time.Now().Sub(start)
	ZapLogger.Info("read file from cache",
		zap.Any("file", bh.LocalName),
		zap.Any("size", cntSize),
		zap.Any("duration seconds", duration.Seconds()))

	return bodyBytes
}

func (bh *BinHeader) readBlob4K(blbId string, offset int64) (blobBody []byte) {
	f, err := os.OpenFile(bh.LocalName, os.O_RDONLY, 0755)
	if err != nil {
		ZapLogger.Fatal("os.OpenFile", zap.Any("file", bh.LocalName), zap.Any("err", err))
	}
	defer f.Close()
	idSizeAndCheckSum := make([]byte, definition.F_BLOBID_SIZE+8+definition.F_CHECKSUM_SIZE)
	if _, err = f.ReadAt(idSizeAndCheckSum, offset); err != nil {
		ZapLogger.Fatal("f.ReadAt failed", zap.Any("err", err))
	}
	idOnDisk := DecodeName(idSizeAndCheckSum[:definition.F_BLOBID_SIZE])
	if strings.Compare(blbId, idOnDisk) != 0 {
		ZapLogger.Fatal("blob name mismatch",
			zap.Any("blobId", blbId),
			zap.Any("idOnDisk", idOnDisk))
	}
	dataSize := DecodeSize(idSizeAndCheckSum[definition.F_BLOBID_SIZE : definition.F_BLOBID_SIZE+8])
	chunksNum := (dataSize + definition.F_CONTENT_SIZE - 1) / definition.F_CONTENT_SIZE
	totalBytes := make([]byte, chunksNum*4*definition.K_KiB)
	if _, err = f.ReadAt(totalBytes, offset); err != nil {
		ZapLogger.Fatal("f.ReadAt failed", zap.Any("err", err))
	}
	blobId, bodyBytes := Decode4K(totalBytes)
	//TODO: Use error return instead, rather than directly crashing the server
	if strings.Compare(blobId, idOnDisk) != 0 {
		ZapLogger.Fatal("blob name mismatch",
			zap.Any("blobId", blobId),
			zap.Any("idOnDisk", idOnDisk))
	}
	return bodyBytes
}

//////////////////////////////////////////////////////
// Below are handy encoding functions for operating
// on the binary data in blob_binary persistent file.

// [   128 byte  ][   8 byte      ][   content   ]
// .....................................................
// |   blob Id   | size of content|     binary   |
// TODO: Goroutine copy. Move these byte operations to a
// separate class.
func Encode(blobId string, data []byte) (encoded []byte) {
	// prepare first part: blobId
	blbIdBytes := []byte(blobId)
	buf1st := new(bytes.Buffer)
	binary.Write(buf1st, binary.LittleEndian, &blbIdBytes)

	// prepare second part: data length
	var length int64 = int64(len(data))
	buf2nd := new(bytes.Buffer)
	binary.Write(buf2nd, binary.LittleEndian, &length)

	// prepare third part: data body
	allBytes := make([]byte, 136+len(data))
	// buf3rd := new(bytes.Buffer)
	// binary.Write(buf3rd, binary.LittleEndian, length)

	// Copy blobId to destination bytes
	copy(allBytes[0:128], buf1st.Bytes())
	// Copy binary length to destination bytes
	copy(allBytes[128:136], buf2nd.Bytes())
	// Copy data to destination bytes
	copy(allBytes[136:], data)

	return allBytes
}

func Decode(encoded []byte) (blobId string, data []byte) {
	blbId := DecodeName(encoded[0:128])
	size := DecodeSize(encoded[128:136])

	dataBytes := make([]byte, size)
	copy(dataBytes, encoded[136:(136+size)])

	return blbId, dataBytes
}

// TODO implement the checksum.
func Encode4K(blobId string, data []byte) (encoded []byte) {
	chunks := make([]Chunk, 0)
	//get content length
	size := int64(len(data))
	tmpSize := size
	start := 0
	res := new(bytes.Buffer)
	for size > 0 {
		tmp := Chunk{}
		blbIdBytes := []byte(blobId)
		//chunk.blobId
		copy(tmp.ChunkHeader.BlobId[:], blbIdBytes)
		//chunk.size
		tmp.ChunkHeader.Size = size
		//TODO: fake chunk.checksum
		fakeChecksum := []byte("12345678123456781234567812345678")
		copy(tmp.ChunkHeader.Checksum[:], fakeChecksum)
		//chunk.content
		if int64(start+definition.F_CONTENT_SIZE) > tmpSize {
			copy(tmp.Content[:], data[start:])
		} else {
			copy(tmp.Content[:], data[start:start+definition.F_CONTENT_SIZE])
		}
		chunks = append(chunks, tmp)
		start += definition.F_CONTENT_SIZE
		size -= definition.F_CONTENT_SIZE
	}
	for i := 0; i < len(chunks); i++ {
		binary.Write(res, binary.LittleEndian, &chunks[i])
	}
	return res.Bytes()
}

func Decode4K(encoded []byte) (blobId string, data []byte) {
	chunks := make([]Chunk, 0)
	for i := 0; i < len(encoded); i += 4 * definition.K_KiB {
		buf := bytes.NewReader(encoded[i : i+4*definition.K_KiB])
		tmp := Chunk{}
		err := binary.Read(buf, binary.LittleEndian, &tmp)
		if err != nil {
			ZapLogger.Fatal("binary.Read failed", zap.Any("err", err))
		}
		chunks = append(chunks, tmp)
	}
	blbId := string(chunks[0].ChunkHeader.BlobId[:8])
	size := chunks[0].ChunkHeader.Size
	dataBytes := make([]byte, 0)
	for i := 0; i < len(chunks); i++ {
		dataBytes = append(dataBytes, chunks[i].Content[:]...)
	}
	dataBytes = dataBytes[:size]
	return blbId, dataBytes
}

func DecodeName(encoded []byte) (blobId string) {
	decoded := make([]byte, 128)
	buf := bytes.NewReader(encoded)
	err := binary.Read(buf, binary.LittleEndian, &decoded)
	if err != nil {
		ZapLogger.Fatal("binary.Read failed", zap.Any("err", err))
	}
	// TODO: A bit dirty. refactor the hardcoded 8 number
	return string(decoded[:8])
}

func DecodeSize(encoded []byte) int64 {
	var size int64
	buf := bytes.NewReader(encoded)
	err := binary.Read(buf, binary.LittleEndian, &size)
	if err != nil {
		ZapLogger.Fatal("binary.Read failed", zap.Any("err", err))
	}
	return size
}
