package main

import (
	"bytes"
	"fmt"
	_ "image/jpeg" // jpeg形式のサポート
	_ "image/png"  // png形式のサポート
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// init 関数は不要

func main() {
	router := gin.Default()

	router.GET("/health", healthCheckHandler)
	router.POST("/merge", mergeHandler)

	// TODO: ポート番号を設定可能にする
	router.Run(":8080")
}

func healthCheckHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}

func mergeHandler(c *gin.Context) {
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("フォーム取得エラー: %s", err.Error())})
		return
	}
	files := form.File["files[]"]

	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ファイルがアップロードされていません"})
		return
	}

	pdfDataToMerge := []io.ReadSeeker{}
	// Close するために openedFiles リストで管理
	openedFiles := []multipart.File{}
	defer func() {
		for _, f := range openedFiles {
			f.Close()
		}
	}()

	config := model.NewDefaultConfiguration()

	for _, file := range files {
		ext := strings.ToLower(filepath.Ext(file.Filename))

		src, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("ファイルオープン失敗 %s: %s", file.Filename, err.Error())})
			return
		}
		openedFiles = append(openedFiles, src) // Close リストに追加

		switch ext {
		case ".png", ".jpg", ".jpeg":
			imgData, err := io.ReadAll(src)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("画像データ読み込み失敗 %s: %s", file.Filename, err.Error())})
				return
			}
			src.Close() // ReadAll 後は不要

			pdfBuf := new(bytes.Buffer)
			imgReaders := []io.Reader{bytes.NewReader(imgData)}

			// pdfcpu で画像からPDFへ変換 (メモリ上で実行)
			// NOTE: api.ImportImages のシグネチャはライブラリバージョンにより異なる可能性あり
			//       第一引数=nil(既存PDFなし), 第四引数=nil(デフォルト設定) で試行
			err = api.ImportImages(nil, pdfBuf, imgReaders, nil, config)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("画像からPDFへの変換失敗 %s: %s", file.Filename, err.Error())})
				return
			}
			pdfDataToMerge = append(pdfDataToMerge, bytes.NewReader(pdfBuf.Bytes()))

		case ".pdf":
			pdfData, err := io.ReadAll(src)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("PDFデータ読み込み失敗 %s: %s", file.Filename, err.Error())})
				return
			}
			src.Close() // ReadAll 後は不要

			pdfDataToMerge = append(pdfDataToMerge, bytes.NewReader(pdfData))

		default:
			src.Close() // 未サポート形式でも閉じる
			fmt.Printf("無視 (未サポート形式): %s\n", file.Filename)
		}
	}

	if len(pdfDataToMerge) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "マージ対象の有効なファイルが見つかりません"})
		return
	}

	// PDFマージ処理 (メモリ上で実行)
	mergedPdfBuffer := new(bytes.Buffer)

	err = api.MergeRaw(pdfDataToMerge, mergedPdfBuffer, false, config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("PDFマージ失敗: %s", err.Error())})
		return
	}

	// マージ結果をレスポンスとして返す
	c.Header("Content-Disposition", `attachment; filename="merged_output.pdf"`)
	c.Data(http.StatusOK, "application/pdf", mergedPdfBuffer.Bytes())

}
