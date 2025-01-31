package file

import (
	"Spark/client/config"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strconv"

	"github.com/imroc/req/v3"
)

type File struct {
	Name string `json:"name"`
	Size uint64 `json:"size"`
	Time int64  `json:"time"`
	Type int    `json:"type"` // 0: file, 1: folder, 2: volume
}

// listFiles returns files and directories find in path.
func listFiles(path string) ([]File, error) {
	result := make([]File, 0)
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}
	for i := 0; i < len(files); i++ {
		itemType := 0
		if files[i].IsDir() {
			itemType = 1
		}
		result = append(result, File{
			Name: files[i].Name(),
			Size: uint64(files[i].Size()),
			Time: files[i].ModTime().Unix(),
			Type: itemType,
		})
	}
	return result, nil
}

// FetchFile saves file from bridge to local.
// Save body as temp file and when done, rename it to file.
func FetchFile(dir, file, bridge string) error {
	url := config.GetBaseURL(false) + `/api/bridge/pull`
	client := req.C().DisableAutoReadResponse()
	resp, err := client.R().SetQueryParam(`bridge`, bridge).Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// If dest file exists, write to temp file first.
	dest := path.Join(dir, file)
	tmpFile := dest
	destExists := false
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		tmpFile = getTempFileName(dir, file)
		destExists = true
	}

	fh, err := os.Create(tmpFile)
	if err != nil {
		return err
	}
	for {
		buf := make([]byte, 1024)
		n, err := resp.Body.Read(buf)
		if err != nil && err != io.EOF {
			fh.Truncate(0)
			fh.Close()
			os.Remove(tmpFile)
			return err
		}
		if n == 0 {
			break
		}
		_, err = fh.Write(buf[:n])
		if err != nil {
			fh.Truncate(0)
			fh.Close()
			os.Remove(tmpFile)
			return err
		}
		fh.Sync()
	}
	fh.Close()

	// Delete old file if exists.
	// Then rename temp file to file.
	if destExists {
		os.Remove(dest)
		err = os.Rename(tmpFile, dest)
	}
	return err
}

func RemoveFile(path string) error {
	if path == `\` || path == `/` || len(path) == 0 {
		return errors.New(`${i18n|fileOrDirNotExist}`)
	}
	err := os.RemoveAll(path)
	if err != nil {
		return err
	}
	return nil
}

func UploadFile(path, bridge string, start, end int64) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	reader, writer := io.Pipe()
	defer file.Close()
	uploadReq := req.R()
	stat, err := file.Stat()
	if err != nil {
		return err
	}
	size := stat.Size()
	headers := map[string]string{
		`FileName`: stat.Name(),
		`FileSize`: strconv.FormatInt(size, 10),
	}
	if size < end {
		return errors.New(`${i18n|invalidFileSize}`)
	}
	if end == 0 {
		uploadReq.RawRequest.ContentLength = size - start
	} else {
		uploadReq.RawRequest.ContentLength = end - start
	}
	shouldRead := uploadReq.RawRequest.ContentLength
	file.Seek(start, 0)
	go func() {
		for {
			bufSize := int64(2 << 14)
			if shouldRead < bufSize {
				bufSize = shouldRead
			}
			buffer := make([]byte, bufSize) // 32768
			n, err := file.Read(buffer)
			buffer = buffer[:n]
			shouldRead -= int64(n)
			writer.Write(buffer)
			if n == 0 || shouldRead == 0 || err != nil {
				break
			}
		}
		writer.Close()
	}()
	url := config.GetBaseURL(false) + `/api/bridge/push`
	_, err = uploadReq.
		SetBody(reader).
		SetHeaders(headers).
		SetQueryParam(`bridge`, bridge).
		Send(`PUT`, url)
	reader.Close()
	return err
}

func getTempFileName(dir, file string) string {
	exists := true
	tempFile := ``
	for i := 0; exists; i++ {
		tempFile = path.Join(dir, file+`.tmp.`+strconv.Itoa(i))
		_, err := os.Stat(tempFile)
		if os.IsNotExist(err) {
			exists = false
		}
	}
	return tempFile
}
