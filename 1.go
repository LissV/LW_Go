package main

import (
	"archive/zip"
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

func main() {
	archName := "arch1.zip"
	mypath := "C:/Users/admin/Desktop/pr"
	err := createSZP(mypath, archName)
	if err != nil {
		log.Fatal(err)
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

	err = zipWriter.Close()
	if err != nil {
		return err
	}

	data := buf.Bytes()
	err = ioutil.WriteFile(archivename, data, 777)
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
			newPath := filepath.Join(oldNewPath, files[i].Name())
			err = getFiles(path, oldpath, newPath, zipWriter)
			if err != nil {
				return err
			}
		} else {
			path = filepath.Join(oldpath, files[i].Name())
			newPath := filepath.Join(oldNewPath, files[i].Name())
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

func packageFile(path string, fileReader *os.File, zipWriter *zip.Writer) error {
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
