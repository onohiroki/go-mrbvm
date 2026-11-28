# バンドルと単一バイナリ（gomrbpack）

`gomrbpack` は複数の Ruby ソース、RITE バイトコード、データファイルを
1 つの `.mrbb` アーカイブにまとめ、そのアーカイブを埋め込んだ単一の
実行ファイルをビルドするコマンドである。

Ruby の `require` / `load` / `require_relative` はアーカイブ内の
バーチャルファイルシステムを参照し、`File.read` などの読み取りも
同梱データを優先する。実行時にスクリプトや設定ファイルを配布物へ
同梱する必要はない。

## 位置づけ

- `gomruby` — ソースまたは `.mrb` をその場で実行する開発用ランナー
- `gomirb` — 対話 REPL
- `gomrbpack` — アプリケーションを `.mrbb` にまとめ、実行ファイルを作る

`gomrbpack run` と `gomruby app.mrbb` は同じ `bundle` ランタイムを使う。
ただし開発用ランナーである両者はコンパイラと広い capability を渡すため、
`eval` や `File` が使える点が生成バイナリと異なる。

## 使い方

```sh
# 1 つのアーカイブを作る（data/ は File.read 用の同梱データ）
gomrbpack pack --entry main.rb -I lib --include data -o app.mrbb main.rb lib/

# アーカイブを実行する
gomrbpack run app.mrbb arg1 arg2
gomruby app.mrbb arg1 arg2

# 単一の実行ファイルを作る
gomrbpack build --entry main.rb -I lib --include data -o app main.rb lib/
./app arg1 arg2

# eval も使えるようにする（go-mrbc / Prism をリンク）
gomrbpack build --with-compiler -o app-with-eval main.rb
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
- `--include PATH` で任意のファイルやディレクトリを**データ**として
  同梱できる（繰り返し可、glob 可）。位置引数は従来どおりコード専用で、
  同じファイルをコードとデータの両方に指定するとエラーになる。
- アーカイブの entry は必ず実行可能なコード（`rb` / `mrb`）でなければ
  ならない。データだけのアーカイブは作れない。

`-I` は `require` の検索パスとして manifest に記録され、実行時は
`$LOAD_PATH` に反映される。

`gomruby` も `-I` を受け付ける。

```sh
gomruby -I lib main.rb
```

## require の解決

アーカイブ実行時、`require "foo"` は次の順に探す。

1. バンドルルートの `foo`, `foo.rb`, `foo.mrb`
2. manifest の `load_paths` 各ディレクトリの `foo`, `foo.rb`, `foo.mrb`

`require_relative` は読み込み元ファイルのディレクトリだけを探す。
一度 require したファイルは正規化されたアーカイブパスで記録され、2 回目
以降は `false` を返す。

`-I` と manifest の `load_paths` は `$LOAD_PATH` を初期化する。
Ruby 側で `$LOAD_PATH << "lib"` のように変更すると、次の `require` から
そのディレクトリも検索される。

`SourceFS` 使用時はホスト OS のファイルへのフォールバックは行わない。
絶対パスや `..` でルート外へ出るパスは `LoadError` になる。

## 実行時のセマンティクス

- `ARGV` は実行ファイルに渡した引数（`os.Args[1:]`）。
- `$0` は entry のアーカイブパス（例: `main.mrb`）。
- 既定の capability は `require` と `load` のみ。ホストの `File`、
  ネットワーク、プロセスは拒否される。
- 同梱データの `File.read` / `File.exist?` / `File.size` は
  capability なしで読める（アプリの一部として信頼する）。同じパスが
  ホストにもある場合は同梱データが優先される。`File.open` はホスト専用で、
  同梱データには未対応。
- 生成バイナリは既定でコンパイラを含まないため、`eval` やソースの
  `load` は失敗する。`gomrbpack build --with-compiler` を使うと
  go-mrbc をリンクし、`eval` と `sandbox.Eval` を許可したバイナリを作れる。
- 未捕捉例外はバックトレース付きで stderr に表示され、終了コードは 1。
- `gomruby` は拡張子 `.mrbb` に加えて ZIP のマジックでもバンドルを判定する。
  リネームしたアーカイブも実行できる。

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
        os.Stdin, os.Stdout, os.Stderr, bundle.Options{
            Setup: func(state *mrbvm.State) error {
                state.DefineFunction("host_log", hostLog)
                return nil
            },
        }))
}
```

`bundle.Options.Setup` は `State` 作成後、`$0` / `ARGV` / `$DEBUG` の
初期化後、entry 実行前に呼ばれる。ここで `DefineFunction` /
`DefineClass` / `DefineConst` を使ってホスト API を登録する。

`bundle.Options.Configure` は `mrbvm.Options` の実行制限や
`SourceFS` / `AssetFS` を差し替える。`Setup` は `State` 自体を受け取る。

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
    },
    {
      "path": "data/config.json",
      "kind": "data",
      "sha256": "…",
      "size": 128
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
- `--with-compiler` を指定すると、生成する `main.go` が `go-mrbc` を
  リンクして `eval` を有効にする。`go.mod` への追加は `go mod tidy` が
  解決する。バイナリサイズは大きくなる。
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
出力が一致することを確認する。夜間の Bench ワークフローでは
`make bundle-bench` を実行し、通常のベンチマークと一緒に artifact へ
保存する。

## 制限と注意

- 既定の生成バイナリには `eval` 用のコンパイラが入らない。
  `--with-compiler` を使うか、自前の Go ラッパーで
  `bundle.Options.Compiler` を設定する。
- 同梱データは `File.read` / `exist?` / `size` のみ対応。`File.open`、
  ディレクトリ一覧、書き込みはホスト FS のまま（capability が必要）。
- 自己展開スタブ（既存バイナリへアーカイブを追記する方式）は
  採用していない。macOS の署名が壊れるため、`go build` 方式を正とする。
