package common

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// get account type according to address, 0 for transaction/normal account, 1 for contract, -1 for error
func GetAccountType(address string) int {
	if strings.HasSuffix(address, "1") {
		return 0
	} else if strings.HasSuffix(address, "2") {
		return 1
	}
	return -1
}

// Unzip unzips a zip file to a specified destination directory.
func UnZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)

		// Check the security of the file path
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", fpath)
		}

		if f.FileInfo().IsDir() {
			// Create a Directory
			os.MkdirAll(fpath, os.ModePerm)
		} else {
			// Create the directory where the file is located
			if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
				return err
			}

			// Opening a file
			outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer outFile.Close()

			// Open a file in a zip file
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()

			// Copy file contents
			_, err = io.Copy(outFile, rc)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func HexAddrToSAddr(addr string) string {
	if strings.HasPrefix(addr, "0x") {
		addr = strings.Replace(addr, "0x", "", 1)
		addr = strings.ToLower(addr)
		if strings.HasPrefix(addr, "01") {
			addr = "1S" + addr
		} else if strings.HasPrefix(addr, "02") {
			addr = "2S" + addr
		} else if strings.HasPrefix(addr, "03") {
			addr = "3S" + addr
		} else if strings.HasPrefix(addr, "04") {
			addr = "4S" + addr
		} else {
			addr = "0x" + addr
		}
	}
	return addr
}
