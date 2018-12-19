package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/fullsailor/pkcs7"
)

type fileInfo struct {
	fileName  string    `json:"name"`
	origSize  int64     `json:"originalSize"`
	lastModif time.Time `json:"lastModificationTime"`
	hash      [20]byte  `json:"hash"`
}

var metArray = make([]fileInfo, 0)
var temp = make([]byte, 0)

func main() {
	var mode string
	var path string
	var name string
	var hash string
	var cert string
	var key string
	flag.StringVar(&name, "name", "archive.szp", "Name your archive")
	flag.StringVar(&mode, "mode", "", "Choose mode: z - zip file(s), i - get information about zipped files and x - extract zipped files")
	flag.StringVar(&path, "path", "./", "Locate your file or your directoty")
	flag.StringVar(&hash, "hash", "", "Hash of your archive")
	flag.StringVar(&cert, "cert", "./", "Your certificate")
	flag.StringVar(&key, "pkey", "./", "Your key")
	flag.Parse()

	name = name + ".szp"
	switch mode {
	case "z":
		err := createSZP(path, name, cert, key)
		if err != nil {
			log.Fatal(err)
		}
	case "i":
		sign, err := checkSecurity(name, cert, key, hash)
		if err != nil {
			log.Fatal(err)
		}

		metaReader := bytes.NewReader(sign.Content)
		metaInBytes := make([]byte, 0)
		off := int64(0)
		_, err = metaReader.ReadAt(metaInBytes, off)

		meta, err := readMeta(metaInBytes)
		if err != nil {
			log.Fatal(err)
		}

		for _, info := range meta {
			fmt.Printf("%+v\n", info)
		}

	case "x":
		if name == "F" {
			fmt.Println("Archive wasn't found")
			os.Exit(-1)
		}

		err := extract(name, path, cert, key, hash)
		if err != nil {
			fmt.Println(err)
			os.Exit(-1)
		}

		fmt.Printf("Unpacked successfully!\n")
	}
}

func createSZP(srcpath, archivename string, cert string, key string) error {
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)
	dest := "./"

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
	err = binary.Write(finalZippedDataWriter, binary.LittleEndian, zippedFiles)
	if err != nil {
		return err
	}

	err = getFinalArchive(finalZippedDataWriter.Bytes(), dest, archivename, cert, key)
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
			// newPath - сокращенный путь к файлу для создания файловой системы внутри каталога,
			// в newPath содержится путь, начиная с требуемого каталога, а не с диска
			path = filepath.Join(oldpath, files[i].Name())
			newPath := filepath.Join(oldNewPath, files[i].Name())
			err = getFiles(path, oldpath, newPath, zipWriter)
			if err != nil {
				return err
			}
		} else {
			path = filepath.Join(oldpath, files[i].Name())
			newPath := filepath.Join(oldNewPath, files[i].Name())
			err = addMeta(files[i], &metArray, newPath, path)
			if err != nil {
				return err
			}
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

func addMeta(info os.FileInfo, metaData *[]fileInfo, path string, fullPath string) error {
	file, err := os.Open(fullPath)
	if err != nil {
		return err
	}

	fileBody, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}

	meta := &fileInfo{
		fileName:  path,
		origSize:  info.Size(),
		lastModif: info.ModTime(),
		hash:      sha1.Sum(fileBody)}
	*metaData = append(*metaData, *meta)
	return err
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
	fmt.Println(len(meta))
	binary.LittleEndian.PutUint32(meta4Bytes, uint32(len(meta)))

	err := binary.Write(finalZippedDataWriter, binary.LittleEndian, meta4Bytes)
	if err != nil {
		return err
	}

	return err
}

func readMeta(data []byte) (metaData []fileInfo, err error) {
	metaSize := int32(binary.LittleEndian.Uint32(data[:4]))
	meta, err := zip.NewReader(bytes.NewReader(data[4:metaSize+4]), int64(len(data[4:metaSize+4])))
	if err != nil {
		return nil, err
	}

	file := meta.File[0]
	a, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer a.Close()

	buf := new(bytes.Buffer)

	_, err = io.Copy(buf, a)
	if err != nil {
		log.Printf(err.Error())
		return nil, err
	}

	err = json.Unmarshal(buf.Bytes(), &metaData)
	if err != nil {
		return nil, err
	}

	return metaData, err
}

func checkSecurity(name string, cert string, key string, hash string) (*pkcs7.PKCS7, error) {
	archive, err := ioutil.ReadFile(name)
	if err != nil {
		return nil, err
	}

	sign, signer, err := checkSign(archive, hash)
	if err != nil {
		return nil, err
	}

	err = checkCert(cert, key, signer)
	if err != nil {
		return nil, err
	}

	return sign, err
}

func checkSign(archive []byte, hash string) (*pkcs7.PKCS7, *x509.Certificate, error) {
	sign, err := pkcs7.Parse(archive)
	if err != nil {
		return nil, nil, err
	}

	err = sign.Verify()
	if err != nil {
		return nil, nil, err
	}

	signer := sign.GetOnlySigner()
	if signer == nil {
		return nil, nil, errors.New("There is a problem with signer")
	}

	if hash != "" {
		if hash != fmt.Sprintf("%x", sha1.Sum(signer.Raw)) {
			fmt.Println(fmt.Sprintf("%x", sha1.Sum(signer.Raw)))
			return nil, nil, errors.New("There is a problem with certificate")
		}
	}

	return sign, signer, err
}

func checkCert(cert string, key string, signer *x509.Certificate) error {
	certificate, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return err
	}

	rsaCertificate, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return err
	}

	if bytes.Compare(rsaCertificate.Raw, signer.Raw) != 0 {
		return errors.New("Certificates don't match")
	}

	return err
}

func getFinalArchive(data []byte, dest string, name string, cert string, key string) error {

	signedData, rsaCertificate, err := signData(data, cert, key)
	if err != nil {
		return err
	}

	archive, err := signedData.Finish()
	if err != nil {
		return err
	}

	fmt.Printf("Certificate's hash: %x\n", sha1.Sum(rsaCertificate.Raw))

	finalArchive, err := os.Create(filepath.Join(dest, name))
	if err != nil {
		return err
	}
	defer finalArchive.Close()

	_, err = finalArchive.Write(archive)
	if err != nil {
		return err
	}

	return nil
}

func signData(data []byte, cert string, key string) (*pkcs7.SignedData, *x509.Certificate, error) {
	signedData, err := pkcs7.NewSignedData(data)
	if err != nil {
		return nil, nil, err
	}

	certificate, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return nil, nil, err
	}

	rsaKey := certificate.PrivateKey
	rsaCertificate, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return nil, nil, err
	}

	err = signedData.AddSigner(rsaCertificate, rsaKey, pkcs7.SignerInfoConfig{})
	if err != nil {
		return nil, nil, err
	}

	return signedData, rsaCertificate, err
}

func extractArchive(zipReader *zip.Reader, metaFiles []fileInfo, path string) error {
	for _, file := range zipReader.File {
		fileContent, err := file.Open()
		if err != nil {
			return err
		}

		fileBody, err := ioutil.ReadAll(fileContent)
		if err != nil {
			return err
		}

		for _, meta := range metaFiles {
			if filepath.Base(meta.fileName) == filepath.Base(file.Name) {
				fileHash := sha1.Sum(fileBody)
				if meta.hash != fileHash {
					return errors.New("Hash damaged")
				}
			}
		}

		err = writeFiles(file, path, fileBody)
		if err != nil {
			return err
		}
		fileContent.Close()
	}
	return nil
}

func writeFiles(file *zip.File, path string, fileBody []byte) (err error) {
	fileInfo := file.FileInfo()
	if fileInfo.IsDir() {
		_, err = os.Stat(filepath.Join(path, file.Name))
		if os.IsNotExist(err) {
			os.MkdirAll(filepath.Join(path, file.Name), os.ModePerm)
		} else {
			return errors.New("Folder " + file.Name + " already exists")
		}
	} else {
		f, err := os.Create(filepath.Join(path, file.Name))
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.Write(fileBody)
		if err != nil {
			return err
		}
	}
	return err
}

func extract(name string, path string, cert string, key string, hash string) error {
	sign, err := checkSecurity(name, cert, key, hash)
	if err != nil {
		return err
	}

	metaReader := bytes.NewReader(sign.Content)
	metaInBytes := make([]byte, 0)
	off := int64(0)
	_, err = metaReader.ReadAt(metaInBytes, off)

	meta, err := readMeta(metaInBytes)
	if err != nil {
		return err
	}

	metaSize := int64(binary.BigEndian.Uint32(sign.Content[:4]))

	bytedArchive := bytes.NewReader(sign.Content[4+metaSize:])

	zipReader, err := zip.NewReader(bytedArchive, bytedArchive.Size())
	if err != nil {
		return err
	}

	err = extractArchive(zipReader, meta, path)
	if err != nil {
		return err
	}
	return nil
}
