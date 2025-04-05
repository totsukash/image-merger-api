# Image/PDF Merger API

Go言語 (Gin + pdfcpu) で実装された、画像とPDFファイルをマージして単一のPDFファイルとして返すシンプルなWeb APIです。

## 概要

このAPIは、アップロードされた複数の画像ファイル (PNG, JPG/JPEG) とPDFファイルを結合し、一つのPDFファイル `merged_output.pdf` としてダウンロードさせます。すべての処理はサーバーのメモリ上で行われ、一時ファイルは使用しません。

## 特徴

- PNG, JPG/JPEG 画像ファイルとPDFファイルの結合をサポート
- すべての処理をメモリ上で完結 (サーバーレス環境などでの利用に適しています)
- シンプルな HTTP POST リクエストで利用可能
- Go言語の標準ライブラリと Gin, pdfcpu ライブラリで実装

## API仕様

### `/merge`

- **メソッド:** `POST`
- **リクエスト形式:** `multipart/form-data`
  - ファイルは `files[]` というキー名で複数指定します。
- **レスポンス:** 
  - **成功時:** `200 OK` と共に、結合されたPDFファイル (`merged_output.pdf`) が `application/pdf` として返されます。`Content-Disposition: attachment; filename="merged_output.pdf"` ヘッダーが付与されます。
  - **失敗時:** エラー内容を示すJSONレスポンス (例: `400 Bad Request`, `500 Internal Server Error`)
- **サポートされるファイル形式:** `.png`, `.jpg`, `.jpeg`, `.pdf`

## 使い方

### cURL を使用した例

```bash
curl -X POST \
  http://localhost:8080/merge \
  -F "files[]=@/path/to/your/image1.png" \
  -F "files[]=@/path/to/your/document.pdf" \
  -F "files[]=@/path/to/your/image2.jpg" \
  --output merged_output.pdf
```

`/path/to/your/` の部分は実際のファイルパスに置き換えてください。

## セットアップと実行

1.  **Go言語環境:** Go 1.x 以降がインストールされている必要があります。
2.  **依存関係のインストール:** プロジェクトディレクトリで以下のコマンドを実行します。
    ```bash
    go mod tidy
    ```
3.  **ビルド:**
    ```bash
    go build
    ```
4.  **実行:**
    ```bash
    ./image-merger-api
    ```
    デフォルトでは `http://localhost:8080` でサーバーが起動します。

## 依存ライブラリ

- [Gin Web Framework](https://github.com/gin-gonic/gin)
- [pdfcpu](https://github.com/pdfcpu/pdfcpu)

## 今後の改善点 (TODO)

- サーバーのポート番号を設定ファイルや環境変数から変更可能にする。 