package utils

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// RenameFile 重命名文件
func RenameFile(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

// getFilePath 函数用于获取文件路径中的目录部分
func GetFilePath(filePath string) string {
	return filepath.Dir(filePath)
}

// DeleteFile 删除文件
func DeleteFile(path string) error {
	return os.Remove(path)
}

func CreateFilePath(path string) string {

	if err := os.MkdirAll(path, os.ModePerm); err != nil {
		return ""
	}
	return path
}

// DeleteFileAndParentDir 删除指定文件及其所在目录（包括目录中的所有内容）
func DeleteFileAndParentDir(filePath string) error {
	// 获取文件的绝对路径
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("获取绝对路径失败: %v", err)
	}

	// 获取父目录路径
	parentDir := filepath.Dir(absPath)

	// 检查父目录是否存在
	if _, err := os.Stat(parentDir); os.IsNotExist(err) {
		return fmt.Errorf("目录不存在: %s", parentDir)
	}

	log.Printf("准备删除目录及其内容: %s", parentDir)

	// 删除整个目录及其内容
	err = os.RemoveAll(parentDir)
	if err != nil {
		return fmt.Errorf("删除目录失败: %v", err)
	}

	log.Printf("成功删除目录: %s", parentDir)
	return nil
}

func CopyFile(src, dst string) error {
	// 打开源文件
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	// 创建目标文件
	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	// 复制文件内容
	_, err = io.Copy(destination, source)
	return err
}
func GetFilePathDir(src string) string {

	return filepath.Dir(src)
}

// 获取不包含扩展名的文件名
func GetFileNameWithoutExtension(filePath string) (string, error) {
	// 检查文件是否存在
	info, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("文件不存在或无法访问: %w", err)
	}

	// 确保是文件（非目录）
	if info.IsDir() {
		return "", fmt.Errorf("指定路径是目录，不是文件: %s", filePath)
	}

	// 获取文件名（包含扩展名）
	filename := filepath.Base(filePath)

	// 处理无扩展名的情况
	if !strings.Contains(filename, ".") {
		return filename, nil
	}

	// 移除扩展名（保留最后一个点后的部分）
	// 例如: "video.mp4" → "video", "clip.mkv.remux" → "clip.mkv"
	return strings.TrimSuffix(filename, filepath.Ext(filename)), nil
}
