package main

import (
	"fmt"
	_ "image/jpeg" // jpeg形式のサポート
	_ "image/png"  // png形式のサポート
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pdfcpu/pdfcpu/pkg/api"          // pdfcpu API をインポート
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model" // pdfcpu 設定用モデルをインポート
)

// init 関数は不要になったため削除

func main() {
	router := gin.Default()

	// ヘルスチェックAPI
	router.GET("/health", healthCheckHandler)

	// 画像/PDF マージ API
	router.POST("/merge", mergeHandler)

	// サーバー起動
	// TODO: ポート番号を設定可能にする
	router.Run(":8080")
}

// healthCheckHandler はヘルスチェックリクエストを処理します。
func healthCheckHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}

// mergeHandler は画像とPDFのマージリクエストを処理します。
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

	// 一時ディレクトリを作成
	tempDir := "./temp_uploads"
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("一時ディレクトリ作成失敗: %s", err.Error())})
		return
	}
	// 関数終了時に一時ディレクトリを削除
	defer os.RemoveAll(tempDir)

	// マージ対象となるPDFファイルのパスリスト
	pdfFilePathsToMerge := []string{}

	// pdfcpuの設定 (デフォルトを使用)
	config := model.NewDefaultConfiguration()
	// 必要であればここでconfigをカスタマイズする
	// 例: config.ValidationMode = model.ValidationRelaxed

	for _, file := range files {
		ext := strings.ToLower(filepath.Ext(file.Filename))
		originalFilePath := filepath.Join(tempDir, file.Filename)

		// アップロードされたファイルを一時ディレクトリに保存
		if err := c.SaveUploadedFile(file, originalFilePath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("ファイル保存失敗 %s: %s", file.Filename, err.Error())})
			return
		}

		switch ext {
		case ".png", ".jpg", ".jpeg":
			// 画像をPDFに変換 (pdfcpu APIを使用)
			imagePdfPath := filepath.Join(tempDir, strings.TrimSuffix(file.Filename, ext)+".pdf")
			// ImportImagesFile は画像ファイルのリストを受け取るので、一つだけのリストを作成
			imageFiles := []string{originalFilePath}
			// 画像インポート設定
			// デフォルト設定を使用
			// imp := &model.ImportConfig{} // pdfcpu v0.5.0 以降の正しい型 - 未定義エラー発生
			// 必要に応じて設定を変更: 例 imp.PageDim = model.PageSizeA4.ToRect()

			// imp に nil を渡し、デフォルト設定を使用する
			err = api.ImportImagesFile(imageFiles, imagePdfPath, nil, config)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("画像からPDFへの変換失敗 %s: %s", file.Filename, err.Error())})
				return
			}
			pdfFilePathsToMerge = append(pdfFilePathsToMerge, imagePdfPath)
			fmt.Printf("画像変換 -> PDF: %s\n", imagePdfPath)

		case ".pdf":
			// 元のPDFをマージリストに追加
			pdfFilePathsToMerge = append(pdfFilePathsToMerge, originalFilePath)
			fmt.Printf("追加 PDF: %s\n", originalFilePath)

		default:
			// サポートされていないファイル形式は無視
			fmt.Printf("無視 (未サポート形式): %s\n", file.Filename)
		}
	}

	if len(pdfFilePathsToMerge) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "マージ対象の有効なファイルが見つかりません"})
		return
	}

	// --- PDFマージ処理 (pdfcpu) ---
	mergedPdfPath := filepath.Join(tempDir, "merged_output.pdf")

	// 出力ファイルを作成 (io.Writer)
	outFile, err := os.Create(mergedPdfPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("出力ファイル作成失敗: %s", err.Error())})
		return
	}
	defer outFile.Close() // 必ず閉じる

	// 入力ファイルを開いて io.ReadSeeker のスライスを作成
	readers := make([]io.ReadSeeker, 0, len(pdfFilePathsToMerge))
	filesToClose := []*os.File{} // 開いたファイルを記録して後で閉じる
	defer func() {
		for _, f := range filesToClose {
			f.Close()
		}
	}()

	for _, pdfPath := range pdfFilePathsToMerge {
		f, err := os.Open(pdfPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("入力PDFファイルオープン失敗 %s: %s", pdfPath, err.Error())})
			return // エラー発生時は開いたファイルを defer で閉じる
		}
		readers = append(readers, f)
		filesToClose = append(filesToClose, f)
	}

	// MergeRaw は []io.ReadSeeker と io.Writer を受け取る
	err = api.MergeRaw(readers, outFile, false, config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("PDFマージ失敗: %s", err.Error())})
		return // エラー発生時は開いたファイルを defer で閉じる
	}

	fmt.Println("PDFマージ完了:", mergedPdfPath)

	// --- レスポンス ---
	// マージされたPDFファイルを返す
	c.FileAttachment(mergedPdfPath, "merged_output.pdf")
}

// addImagePage 関数は不要になったため削除
