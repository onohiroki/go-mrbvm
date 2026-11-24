# go-mrbc / go-mrbvm ツールチェーン

`go-mrbc` と `go-mrbvm` は、pure Go の mruby ツールチェーンを構成する
2 つのリポジトリである。

| リポジトリ | 役割 | 入出力 |
| --- | --- | --- |
| [`go-mrbc`](https://github.com/onohiroki/go-mrbc) | Ruby → バイトコードのコンパイラ | Ruby ソースを読み、RITE 04.00 を出力 |
| [`go-mrbvm`](https://github.com/onohiroki/go-mrbvm) | mruby 仮想マシン | RITE 04.00 を読み、実行 |

どちらも **mruby 4.0.0** と **RITE 04.00** を対象にする。互換性の基準は
引き続き C 実装の mruby 4.0.0 (`mruby` / `mrbc`) である。

## データフロー

```text
Ruby ソース
    |
    v
go-mrbc  (Prism parser → lowering → register compiler → RITE writer)
    |
    v
RITE 04.00 バイトコード
    |
    +-----------------------------+
    |                             |
    v                             v
go-mrbvm                      mruby 4.0.0
(pure Go VM)                  (参照実装)
```

コンパイラは VM なしでバイトコードを生成でき、VM はコンパイラなしで
バイトコードを実行できる。

## 一方向の依存

```text
go-mrbvm ──imports──▶ go-mrbc/bytecode
go-mrbc  ──no dependency on──▶ go-mrbvm
```

VM が import するのは `go-mrbc/bytecode`（RITE のデータモデルと
オペコード codec）だけである。パーサ、lowering、コンパイラには依存しない。

`eval` / `load` / `require` はコンパイラを必要とする操作なので、コアの
`mrbvm` パッケージは `Options.Compiler` でコンパイラを受け取る。これらを
使わない埋め込みではコンパイラをリンクしない。`gomruby` と `gomirb` の
コマンドは Ruby ソースを直接扱うため、両方のモジュールをリンクする。

## 責務の分担

`go-mrbc`:

- Prism AST の lowering
- 内部 IR、スコープ、レジスタ割り当て、コード生成
- RITE 04.00 の writer / reader（`bytecode` パッケージ）
- オペコード定義と命令 codec
- 共有 conformance corpus（`conformance/testdata/conformance`）
- コンパイル互換性・実行時互換性のテスト

`go-mrbvm`:

- バイトコードインタプリタとオブジェクトモデル
- コアクラス、モジュール、言語意味論
- `Regexp` / `MatchData`、`Fiber`、`eval` / `Binding`、`Set`、`Struct`、
  `Data` などのネイティブ mrbgem
- サンドボックスとケイパビリティポリシー
- `mrbvm` パッケージの埋め込み API
- `gomruby` / `gomirb` / `vmcompat` / `mrbtest` / `vmbench`
- 公式 mruby テストのマニフェスト

バイトコード形式や命令の意味を変える変更は両リポジトリにまたがる。
コンパイラ側の変更、VM 側の変更、conformance ケースを同じリリースサイクルで
入れる。

## 共有契約

- RITE 04.00 のコンテナ
- `bytecode.Irep`、pool、symbol、catch handler、debug line
- 命令のエンコード / デコードとオペランド幅
- `go-mrbc` が所有し、両方が実行する conformance corpus

corpus は `go-mrbc` の `conformance` パッケージに埋め込まれ、`go-mrbvm` の
テストからも同じケースが実行される。片方だけ通るコンパイラ変更を防ぐためで
ある。

## ライセンスと依存ファイル

2 つのリポジトリはどちらも MIT License で公開する。ライセンス関連の
ファイルは次のとおり。

| ファイル | 内容 |
| --- | --- |
| `LICENSE` | オリジナルのソースとドキュメントの MIT License |
| `NOTICE` | 配布バイナリ向けの帰属表示 |
| `THIRD_PARTY_LICENSES.md` | コンポーネント、バージョン、ライセンス、著作権 |
| `third_party/` | 同梱コンポーネントのライセンス全文と NOTICE |

直接依存・リンクするコンポーネント:

| コンポーネント | 利用箇所 | ライセンス |
| --- | --- | --- |
| `go-mrbc/bytecode` | `go-mrbvm` コア | MIT |
| [dlclark/regexp2](https://github.com/dlclark/regexp2) | `go-mrbvm` の Regexp/MatchData | MIT |
| [go-ruby-prism](https://github.com/danielgatis/go-ruby-prism) | `go-mrbc`、`gomruby`、`gomirb` | MIT |
| [Prism](https://github.com/ruby/prism)（`prism.wasm` として同梱） | go-ruby-prism 経由 | MIT |
| [wazero](https://github.com/tetratelabs/wazero) | Prism の WebAssembly ランタイム | Apache-2.0 |

mruby (MIT) はバイトコードと VM の仕様、およびテスト入力として参照する。
mruby の C コードはリンクしない。ChibiRuby (MIT) は先行実装の参照のみで、
コードはコピーしていない。

ソース配布では `go.mod`、`go.sum`、`LICENSE`、`NOTICE`、
`THIRD_PARTY_LICENSES.md`、`third_party/` を保持する。バイナリ配布では、
リンクしたコードに対応するライセンスと NOTICE を含める。詳細な
コンポーネント表は各リポジトリの `THIRD_PARTY_LICENSES.md` にある。

## 互換ターゲット

- mruby 4.0.0
- RITE 04.00
- 対応サブセットにおけるデフォルト `mrbc` の出力形式

コンパイラの互換性レポートは
[`compatibility-report.md`](https://github.com/onohiroki/go-mrbc/blob/main/docs/compatibility-report.md)、
VM の対応テストと除外理由は
[`roadmap.ja.md`](https://github.com/onohiroki/go-mrbvm/blob/main/docs/roadmap.ja.md)
にある。

VM の `String` は UTF-8 文字単位のインデックスを使い、byte API は
byte 単位のままとする。これは mruby との意図的な差分であり、公開仕様の
一部である。

## 開発時の配置

開発中は `go-mrbvm/go.mod` が隣接チェックアウトを参照する:

```text
replace github.com/onohiroki/go-mrbc => ../go-mrbc
```

想定する配置:

```text
workspace/
├── go-mrbc/
└── go-mrbvm/
```

CI も 2 つのリポジトリを並べてチェックアウトする。公開リリースでは
`replace` を外し、タグ付けした `go-mrbc` を固定する。手順は
[`releasing.md`](releasing.md) を参照。

## バージョンとリリース順

`go-mrbvm` が `go-mrbc/bytecode` に依存するため、必ず `go-mrbc` を先に
タグ付けする。

1. `go-mrbc` をタグ付け（例: `v0.1.0`）
2. `go-mrbvm` の require をそのタグに更新し、`replace` を削除
3. コンパイラと VM の全ゲートを実行
4. `go-mrbvm` をタグ付け

2 つのリポジトリは同じバージョン番号を共有しない。`go-mrbvm` は
semver 互換な `go-mrbc` を固定する。

## テストの対応

`go-mrbc`:

```sh
go test ./...
go run ./cmd/compat -fail mruby/test mruby/mrbgems
go run ./cmd/runtimecompat -fail -assert mruby/test/assert.rb mruby/test/t mruby/mrbgems
```

`go-mrbvm`:

```sh
go test ./...
go run ./cmd/vmcompat -corpus
go run ./cmd/mrbtest -manifest testdata/official.txt
go run ./cmd/mrbtest -manifest testdata/official-gems.txt
```

mruby 4.0.0 のチェックアウトはテスト入力であり、どちらのモジュールにも
vendoring されない。

## 変更の置き場所

- パーサ、lowering、コンパイラ、bytecode writer、conformance corpus:
  `go-mrbc`
- インタプリタ、コアライブラリ、サンドボックス、埋め込み API、公式テスト:
  `go-mrbvm`
- 命令の意味、RITE レイアウト、共有 conformance の挙動: 両方

パッチのチェックリストは [`../CONTRIBUTING.md`](../CONTRIBUTING.md) を参照。
