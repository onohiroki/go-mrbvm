# バンドルと単一バイナリ（gomrbpack）

`gomrbpack` は複数の Ruby ソースと RITE バイトコードを 1 つの `.mrbb`
アーカイブにまとめ、そのアーカイブを埋め込んだ単一の実行ファイルを
ビルドするコマンドである。

Ruby の `require` / `load` / `require_relative` はアーカイブ内の
バーチャルファイルシステムを参照するため、実行時にスクリプトを
配布物へ同梱する必要はない。

## 位置づけ

- `gomruby` — ソースまたは `.mrb` をその場で実行する開発用ランナー
- `gomirb` — 対話 REPL
- `gomrbpack` — アプリケーションを `.mrbb` にまとめ、実行ファイルを作る

`gomrbpack run` と `gomruby app.mrbb` は同じ `bundle` ランタイムを使う。
ただし開発用ランナーである両者はコンパイラと広い capability を渡すため、
`eval` や `File` が使える点が生成バイナリと異なる。

## 使い方

```sh
# 1 つのアーカイブを作る
gomrbpack pack --entry main.rb -I lib -o app.mrbb main.rb lib/

# アーカイブを実行する
gomrbpack run app.mrbb arg1 arg2
gomruby app.mrbb arg1 arg2

# 単一の実行ファイルを作る
gomrbpack build --entry main.rb -I lib -o app main.rb lib/
./app arg1 arg2
```

`build` は Go ツールチェーンを使って、`app.mrbb` を `//go:embed` した
小さな `main.go` をコンパイルする。生成物は通常の Go バイナリなので、
macOS のコード署名やクロスコンパイルは `go build` の仕組みがそのまま
使える。

## 入力と entry

- 入力は `.rb` と `.mrb`。ディレクトリは再帰的に展開し、glob も使える。
  ディレクトリ内の `.rb` / `.mrb` 以外（README、`.mrbb`、`.git`）は無視され、
  明示的に渡したファイルだけが型を検査される。
- アーカイブ内のパスは `--root`（既定 `.`）からの相対パスになる。
- `--entry` を省略した場合、最初のファイル引数、次に `main.rb` /
  `main.mrb`、最後にソート順の先頭が entry になる。
- `.rb` は**パック時に** RITE 04.00 へコンパイルされる。アーカイブには
  `.mrb` だけが入るため、生成バイナリはコンパイラ（Prism / wazero）を
  リンクしない。
- `.mrb` は読み込み時に `DecodeBytecode` で検証される。

`-I` は `require` の検索パスとして manifest に記録される。

## require の解決

アーカイブ実行時、`require "foo"` は次の順に探す。

1. バンドルルートの `foo`, `foo.rb`, `foo.mrb`
2. manifest の `load_paths` 各ディレクトリの `foo`, `foo.rb`, `foo.mrb`

`require_relative` は読み込み元ファイルのディレクトリだけを探す。
一度 require したファイルは正規化されたアーカイブパスで記録され、2 回目
以降は `false` を返す。

`SourceFS` 使用時はホスト OS のファイルへのフォールバックは行わない。
絶対パスや `..` でルート外へ出るパスは `LoadError` になる。

## 実行時のセマンティクス

- `ARGV` は実行ファイルに渡した引数（`os.Args[1:]`）。
- `$0` は entry のアーカイブパス（例: `main.mrb`）。
- 既定の capability は `require` と `load` のみ。`File`、ネットワーク、
  プロセスは拒否される。
- 生成バイナリはコンパイラを含まないため、`eval` やソースの `load`
  は失敗する。必要な場合は `bundle.Options.Compiler` と
  `sandbox.Eval` を渡して自前のラッパーをビルドする。
- 未捕捉例外はバックトレース付きで stderr に表示され、終了コードは 1。

## Go アプリへの埋め込み

`gomrbpack build` を使わず、既存の Go アプリにバンドルを埋め込むことも
できる。

```go
import (
    "context"
    _ "embed"
    "os"

    "github.com/onohiroki/go-mrbvm/bundle"
)

//go:embed app.mrbb
var archive []byte

func main() {
    os.Exit(bundle.Run(context.Background(), archive, os.Args,
        os.Stdin, os.Stdout, os.Stderr, bundle.Options{}))
}
```

`bundle.Options.Configure` で `mrbvm.Options` の実行制限や
コンパイラを差し替えられる。

`mrbvm.Options.SourceFS` に任意の読み取り専用 FS を渡せる。
`mrbvm.NewSourceFS` は `io/fs.FS`（`embed.FS` や `fstest.MapFS`）を
受け付ける。

```go
state := mrbvm.New(mrbvm.Options{
    SourceFS:  mrbvm.NewSourceFS(embedFS),
    LoadPaths: []string{"lib"},
    Policy:    sandbox.New().Permit(sandbox.Require, sandbox.Load),
})
```

## アーカイブ形式

`.mrbb` は ZIP ファイルで、先頭に `manifest.json` を持つ。

```json
{
  "format": 1,
  "entry": "main.mrb",
  "load_paths": ["lib"],
  "files": [
    {
      "path": "lib/helper.mrb",
      "kind": "mrb",
      "sha256": "…",
      "size": 383
    }
  ]
}
```

- ファイルはパス順に並び、mtime は固定される。同じ入力からは同じ
  バイト列が生成される。
- `Open` は manifest のサイズと SHA-256 を検証し、manifest にない
  メンバーや重複メンバーを拒否する。
- `gomrbpack list` / `info` / `extract` で内容を確認・展開できる。

## build の詳細

`build` は一時ディレクトリに次を生成して `go mod tidy` と `go build`
を実行する。

```text
<workdir>/
├── app.mrbb
├── main.go
└── go.mod
```

- 依存する `go-mrbvm` のバージョンは `gomrbpack` 自身のビルド情報から
  取得する。
- 開発中のチェックアウトを使う場合は `--replace /path/to/go-mrbvm` を
  指定する。この場合バージョンは `v0.0.0` になる。
- `--workdir` を指定すると生成物を残す。指定しない場合は一時
  ディレクトリを使い、終了時に削除する（`--keep-work` で保持）。
- `--goos` / `--goarch` / `--tags` / `--ldflags` / `--strip` /
  `--no-trimpath` を `go build` に渡せる。
- `go` コマンドとモジュールの取得（またはキャッシュ）が必要。

## 計測

`make bundle-bench` は `benchmark/bundle/` のサンプルアプリについて、
起動から終了までの時間とバイナリサイズを計測し、
`benchmark/bundle-latest.txt` に保存する。実装は
`benchmark/bundlebench` にあり、次の 3 通りを同じ出力で比較する。

- `gomruby main.rb` — 起動時にソースをコンパイル
- `gomruby app.mrbb` — パック済みアーカイブを実行
- `built app` — `gomrbpack build` した単体バイナリ

初回の記録は
[`benchmark/bundle-baseline-2026-11-28.txt`](../benchmark/bundle-baseline-2026-11-28.txt)。
Apple M4 / macOS arm64 / go1.27.4、20 回の中央値では次のとおり。

| 実行方法 | 中央値 | 最小 |
| --- | ---: | ---: |
| `gomruby main.rb` | 193.62 ms | 189.57 ms |
| `gomruby app.mrbb` | 6.53 ms | 4.97 ms |
| `built app` | 6.35 ms | 3.56 ms |

サイズは `gomruby` 14.5 MiB、生成バイナリ 8.9 MiB、`app.mrbb` 1.1 KiB。
ランタイム起動はほぼ同じで、差の大部分は起動時のコンパイル時間である。
数値はマシン依存なので、リリースごとに同じ環境で取り直す。

CI では `make bundle-smoke` がサンプルを pack / build / 実行し、3 通りの
出力が一致することを確認する。

## 制限と注意

- 生成バイナリには `eval` 用のコンパイラが入らない。必要なら
  自前の Go ラッパーで `bundle.Options.Compiler` を設定する。
- ホスト FS は既定で使えない。データファイルを同梱したい場合は、
  現時点では Ruby のコードとして埋め込むか、ホスト API を追加する。
- 自己展開スタブ（既存バイナリへアーカイブを追記する方式）は
  採用していない。macOS の署名が壊れるため、`go build` 方式を正とする。
