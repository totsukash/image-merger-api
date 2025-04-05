package main

import (
	"bytes" // bytes パッケージをインポート
	"fmt"
	_ "image/jpeg" // jpeg形式のサポート
	_ "image/png"  // png形式のサポート
	"io"
	"mime/multipart" // multipart.FileHeader のためにインポート
	"net/http"       // エラー処理のために残すが、ファイル操作は減らす
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

	// マージ対象となるPDFデータの io.ReadSeeker スライス
	pdfDataToMerge := []io.ReadSeeker{}
	// アップロードされたファイルのReaderを保持するスライス (後で閉じるため)
	openedFiles := []multipart.File{}
	defer func() {
		for _, f := range openedFiles {
			f.Close()
		}
	}()

	// pdfcpuの設定 (デフォルトを使用)
	config := model.NewDefaultConfiguration()

	for _, file := range files {
		ext := strings.ToLower(filepath.Ext(file.Filename))
		// originalFilePath := filepath.Join(tempDir, file.Filename) // 不要

		// アップロードされたファイルをメモリで開く
		src, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("ファイルオープン失敗 %s: %s", file.Filename, err.Error())})
			return // 開いたファイルを defer で閉じる
		}
		openedFiles = append(openedFiles, src) // Close するためにリストに追加

		// ファイル保存処理を削除
		// if err := c.SaveUploadedFile(file, originalFilePath); err != nil { ... }

		switch ext {
		case ".png", ".jpg", ".jpeg":
			// 画像データを読み込む
			imgData, err := io.ReadAll(src)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("画像データ読み込み失敗 %s: %s", file.Filename, err.Error())})
				return
			}
			// src を閉じる (ReadAll で読み終わったため)
			src.Close() // Close openedFiles スライスからは削除しない (deferで Close するため問題ない)

			// 画像をメモリ上でPDFに変換 (pdfcpu APIを使用)
			// imagePdfPath := filepath.Join(tempDir, strings.TrimSuffix(file.Filename, ext)+".pdf") // 不要
			pdfBuf := new(bytes.Buffer) // 変換後のPDFデータを格納するバッファ

			// ImportImages は map[string]io.Reader を受け取る <- 修正: []io.Reader を使う
			// imageReaders := map[string]io.Reader{
			// 	file.Filename: bytes.NewReader(imgData), // ファイル名をキー、データリーダーを値とする
			// }
			// 画像データを Reader のスライスに入れる
			imgReaders := []io.Reader{bytes.NewReader(imgData)}

			// imp にデフォルト設定の構造体リテラルを渡す <- 修正: nil を使う
			// imp := &model.Import{} // デフォルト設定の構造体リテラル
			// err = api.ImportImages(imageReaders, pdfBuf, imp, config) // imp を渡す <- 修正: シグネチャ変更

			// api.ImportImages を試す (第一引数 nil, 第四引数 imp nil)
			// Linterが期待するシグネチャに合わせる試み:
			// ImportImages(rs io.ReadSeeker, w io.Writer, readers []io.Reader, imp *Import, conf *model.Configuration)
			err = api.ImportImages(nil, pdfBuf, imgReaders, nil, config)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("画像からPDFへの変換失敗 %s: %s", file.Filename, err.Error())})
				return
			}
			// 変換後のPDFデータ (bytes.Buffer) から io.ReadSeeker を作成してリストに追加
			pdfDataToMerge = append(pdfDataToMerge, bytes.NewReader(pdfBuf.Bytes()))
			fmt.Printf("画像変換 -> メモリPDF: %s\n", file.Filename)

		case ".pdf":
			// PDFデータを読み込む
			pdfData, err := io.ReadAll(src)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("PDFデータ読み込み失敗 %s: %s", file.Filename, err.Error())})
				return
			}
			// src を閉じる
			src.Close()

			// メモリ上のPDFデータから io.ReadSeeker を作成してマージリストに追加
			pdfDataToMerge = append(pdfDataToMerge, bytes.NewReader(pdfData))
			fmt.Printf("追加 メモリPDF: %s\n", file.Filename)

		default:
			// サポートされていないファイル形式は無視
			// src を閉じる (エラーにはしない)
			src.Close()
			fmt.Printf("無視 (未サポート形式): %s\n", file.Filename)
		}
	}

	if len(pdfDataToMerge) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "マージ対象の有効なファイルが見つかりません"})
		return
	}

	// --- PDFマージ処理 (メモリ上で実行) ---
	// mergedPdfPath := filepath.Join(tempDir, "merged_output.pdf") // 不要
	mergedPdfBuffer := new(bytes.Buffer) // マージ結果を格納するバッファ

	// 出力ファイル作成処理を削除
	// outFile, err := os.Create(mergedPdfPath) ...
	// defer outFile.Close()

	// 入力ファイルを開く処理を削除 (既に pdfDataToMerge に ReadSeeker がある)
	// readers := make([]io.ReadSeeker, 0, len(pdfFilePathsToMerge)) ...
	// filesToClose := []*os.File{} ...
	// defer func() { ... }()
	// for _, pdfPath := range pdfFilePathsToMerge { ... }

	// MergeRaw は []io.ReadSeeker と io.Writer を受け取る
	// 入力は pdfDataToMerge、出力は mergedPdfBuffer
	err = api.MergeRaw(pdfDataToMerge, mergedPdfBuffer, false, config) // 引数を修正
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("PDFマージ失敗: %s", err.Error())})
		return // openedFiles の defer Close は実行される
	}

	fmt.Println("PDFマージ完了: メモリ上")

	// --- レスポンス ---
	// マージされたPDFデータをメモリから直接返す
	// c.FileAttachment(mergedPdfPath, "merged_output.pdf") // ファイルパス指定を削除

	// レスポンスヘッダーを設定
	c.Header("Content-Disposition", `attachment; filename="merged_output.pdf"`)
	c.Data(http.StatusOK, "application/pdf", mergedPdfBuffer.Bytes())

}

// addImagePage 関数は不要になったため削除
