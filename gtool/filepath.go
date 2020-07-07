package gtool

import (
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"path/filepath"
)

func GetCurrentDir() string {
	curFilename := os.Args[0]
	Path, err := exec.LookPath(curFilename)
	if err != nil {
		log.Error(err)
	}
	binaryPath, err := filepath.Abs(Path)
	if err != nil {
		log.Error(err)
	}
	dir := filepath.Dir(binaryPath)
	return filepath.ToSlash(dir)
}

func GetAbsPath(relativePath string) string {
	relativePath = filepath.ToSlash(relativePath)
	absDir := GetCurrentDir()
	absPath := filepath.Join(absDir, relativePath)
	return filepath.ToSlash(absPath)
}
