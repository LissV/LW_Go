package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"
)

type fileInfo struct {
	filename  string    `json:"Name"`
	origsize  int64     `json:"Original_size"`
	lastModif time.Time `json:"Last_modification_time"`
}

var metArray = make([]fileInfo, 0)

func main() {
	var mode string
	var path string
	var name string
	flag.StringVar(&name, "name", "", "Name your archive")
	flag.StringVar(&mode, "mode", "", "Choose mode: z - zip file(s), i - get information about zipped files and x - extract zipped files")
	flag.StringVar(&path, "path", "", "Locate your file or your directoty")
	flag.Parse()

	//mypath := "C:/Users/admin/Desktop/pr"
	name = name + ".zip"
	switch mode {
	case "z":
		err := createSZP(path, name)
		if err != nil {
			log.Fatal(err)
		}
	case "i":
		log.Println("Info!")

	case "x":
		log.Println("Haven't done yet!")
	}
}

func createSZP(srcpath, archivename string) error {
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	// добавить случай, когда не директория
	var oldpath string
	lastElement := filepath.Base(srcpath)
	err := getFiles(srcpath, oldpath, lastElement, zipWriter)
	if err != nil {
		return err
	}

	var zippedFiles []byte

	err = zipData(zipWriter, &zippedFiles, buf)
	if err != nil {
		return err
	}

	finalZippedDataWriter := new(bytes.Buffer)
	err = getZippedMetadata(finalZippedDataWriter)
	if err != nil {
		return err
	}

	err = binary.Write(finalZippedDataWriter, binary.LittleEndian, zippedFiles)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(archivename, zippedFiles, 777)
	if err != nil {
		return err
	}
	return err
}

func getFiles(path, oldpath, oldNewPath string, zipWriter *zip.Writer) error {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return err
	}
	oldpath = path
	for i := range files {
		if files[i].IsDir() {
			path = filepath.Join(oldpath, files[i].Name())
			// newPath - сокращенный путь к файлу для создания файловой системы внутри каталога,
			// в newPath содержится путь, начиная с требуемого каталога, а не с диска
			newPath := filepath.Join(oldNewPath, files[i].Name())
			err = getFiles(path, oldpath, newPath, zipWriter)
			if err != nil {
				return err
			}
		} else {
			path = filepath.Join(oldpath, files[i].Name())
			newPath := filepath.Join(oldNewPath, files[i].Name())
			addMeta(files[i], &metArray, newPath)
			err = prepareFile(path, zipWriter, newPath)
			if err != nil {
				return err
			}
		}
	}
	return err
}

func prepareFile(file string, zipWriter *zip.Writer, newPath string) error {
	newFile, err := os.Open(file)
	if err != nil {
		return err
	}
	err = packageFile(newPath, newFile, zipWriter)
	if err != nil {
		return err
	}
	err = newFile.Close()
	if err != nil {

		return err
	}
	return err
}

func packageFile(path string, fileReader io.Reader, zipWriter *zip.Writer) error {
	zipFile, err := zipWriter.Create(path)
	if err != nil {
		return err
	}

	_, err = io.Copy(zipFile, fileReader)
	if err != nil {
		return err
	}
	return err
}

func addMeta(info os.FileInfo, metaData *[]fileInfo, path string) {
	meta := &fileInfo{
		filename:  path,
		origsize:  info.Size(),
		lastModif: info.ModTime()}
	*metaData = append(*metaData, *meta)
}

func metaToJSON(meta []fileInfo) (metaJS []byte, err error) {
	return json.Marshal(meta)
}

func zipData(zipWriter *zip.Writer, data *[]byte, dataWriter *bytes.Buffer) error {
	err := zipWriter.Close()
	if err != nil {
		return err
	}

	*data = dataWriter.Bytes()
	return err
}

func getZippedMetadata(finalZippedDataWriter io.Writer) error {
	metaJS, err := metaToJSON(metArray)
	if err != nil {
		return err
	}

	var zippedMeta []byte
	metaBuf := new(bytes.Buffer)
	metaWriter := zip.NewWriter(metaBuf)

	err = packageFile("metadata.json", bytes.NewReader(metaJS), metaWriter)
	if err != nil {
		return err
	}

	err = zipData(metaWriter, &zippedMeta, metaBuf)
	if err != nil {
		return err
	}

	err = getMetaLenInBytes(zippedMeta, finalZippedDataWriter)
	if err != nil {
		return err
	}

	err = binary.Write(finalZippedDataWriter, binary.LittleEndian, zippedMeta)
	if err != nil {
		return err
	}

	return err
}

func getMetaLenInBytes(meta []byte, finalZippedDataWriter io.Writer) error {
	meta4Bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(meta4Bytes, uint32(len(meta)))

	err := binary.Write(finalZippedDataWriter, binary.LittleEndian, meta4Bytes)
	if err != nil {
		return err
	}

	return err
}
