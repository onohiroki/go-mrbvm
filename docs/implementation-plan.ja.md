# go-mrbvm 実装計画

このドキュメントは `go-mrbvm` の実装計画である。英語の設計資料は
[architecture.md](architecture.md)、RITE 04.00 の仕様は
[bytecode-format.md](bytecode-format.md) を参照。

## 1. 目的

- `github.com/onohiroki/go-mrbvm` として、**pure Go** の mruby 4.0.0 VM を実装する。
- 入力は **RITE 04.00**。
  - 別リポジトリ `go-mrbc` が生成した `.mrb`
  - 本家 mruby 4.0.0 の `mrbc` が生成した `.mrb`
- Go ツールのマクロ言語として埋め込めるホスト API を提供する。
- `gomruby`（mruby 相当）と `gomirb`（mirb 相当）を提供する。
- cgo やネイティブ依存を持たない。

## 2. 非目標

- mruby の C API（`mrb_*`）や C 拡張 ABI の互換。
- mruby 3.x など、RITE 04.00 以外のバイトコード。
- CRuby 完全互換。
- JIT。
- C 実装の mrbgem をそのまま動かすこと。
- 1 つの `State` を複数 goroutine から同時利用すること。
- `go-mrbc` の再実装。VM はバイトコードを消費するだけ。

## 3. 前提

- GC は **Go の GC に任せる**。
  - 独自 mark-sweep は実装しない。
  - `ObjectSpace`、finalizer、`GC.start` などは Go GC の上にシムとして実装する。
  - `GC.interval_ratio` など C mruby 固有の内部パラメータは、値を保持して
    引数検証だけ合わせる。完全一致は目指さない。
- 本家 `mrbc` のデフォルト出力は最適化 opcode を含む。
  - そのため VM は **119 opcode すべて**を実装する。
  - `--no-optimize` の `SEND` 中心出力も同じ VM で動く。
- 1 `State` は 1 goroutine が所有する。
- `context.Context` によるキャンセルを命令境界で確認する。

## 4. リポジトリ構成

```text
go-mrbvm/
├── mrbvm/                    公開 API
│   ├── state.go              State, Options, Load, Execute
│   ├── value.go              Value, 型変換
│   ├── class.go              Class, Module, DefineMethod
│   ├── error.go              Ruby 例外 ↔ Go error
│   └── capability.go         ホスト機能の登録
├── internal/
│   ├── vm/                   インタプリタ、フレーム、opcode
│   ├── object/               String, Array, Hash, Proc, Exception
│   ├── core/                 Object, Kernel, Integer, String, Array, Hash
│   ├── gc/                   オブジェクトレジストリ、GC シム
│   └── bytecode/             go-mrbc/bytecode のアダプタ
├── sandbox/                  命令数・時間・深さ・能力制限
├── cmd/
│   ├── gomruby/              mruby 相当
│   └── gomirb/               mirb 相当
├── testdata/conformance/     共有適合コーパス
├── examples/
└── docs/
```

依存方向:

```text
go-mrbvm ──imports──▶ go-mrbc/bytecode
go-mrbc  ──no dependency on──▶ go-mrbvm
```

## 5. 値とオブジェクトモデル

最初は GC フレンドリーなタグ付き構造体にする。

```go
type Kind uint8

type Value struct {
    kind Kind
    bits uint64 // int64, float64 bits, symbol ID
    obj  *Object
}
```

- 即値: `nil`, `false`, `true`, `Integer`, `Float`, `Symbol`
- ヒープ: `String`, `Array`, `Hash`, `Range`, `Proc`, `Class`, `Module`,
  `Exception`, `Data`
- `Object` は class ポインタと ivars を持つ。
- Go GC がポインタを追える形を優先し、NaN boxing は最初は使わない。
- `internal/gc` にオブジェクトレジストリを置き、`ObjectSpace`、
  finalizer、サンドボックス会計に使う。

## 6. 実行モデル

### 6.1 ロード

- `bytecode.ReadBinary` で RITE 04.00 を読む。
- `Irep`、pool、symbol、catch handler、LVAR/DBG を保持。
- `bytecode.Decode` で命令列をデコードしてキャッシュ。
- 本家 `mrbc` の出力も同じ経路で読めることを最初に確認する。

### 6.2 フレーム

- `self`
- `irep`
- `pc`
- レジスタウィンドウ
- ローカル/スタック
- upvar（クロージャ環境）
- 現在の block
- `super` 用の method ID / target class
- catch handler スタック
- backtrace 用の call info

### 6.3 opcode

119 opcode すべてを実装する。主なグループ:

- ロード: `LOADL`, `STRING`, `LOADSYM`, `LOADI_*`, `LOADNIL`, `LOADSELF`
- 変数: `GETGV/SETGV`, `GETIV/SETIV`, `GETCV/SETCV`,
  `GETCONST/SETCONST`, `GETMCNST/SETMCNST`, `GETUPVAR/SETUPVAR`
- 呼び出し: `SEND`, `SENDB`, `SSEND`, `SSENDB`, `SEND0`, `SSEND0`,
  `CALL`, `BLKCALL`
- 引数: `ENTER`, `ARGARY`, `BLKPUSH`, `KEY_P`, `KARG`, `KEYEND`
- ブロック: `BLOCK`, `LAMBDA`, `METHOD`
- クラス: `CLASS`, `MODULE`, `EXEC`, `OCLASS`, `SCLASS`, `TCLASS`,
  `DEF`, `TDEF`, `SDEF`
- 制御: `JMP`, `JMPIF`, `JMPNOT`, `JMPNIL`, `JMPUW`, `EXCEPT`,
  `RESCUE`, `RAISEIF`, `MATCHERR`
- 最適化: `ADD`, `ADDI`, `SUB`, `SUBI`, `MUL`, `DIV`, `EQ`, `LT`,
  `LE`, `GT`, `GE`, `GETIDX`, `SETIDX`, `ARRAY2`, `ARYPUSH`,
  `ARYCAT`, `ARYSPLAT`, `APOST`, `HASHADD`, `HASHCAT`
- `EXT1/EXT2/EXT3`, `NOP`, `DEBUG`, `ERR`, `STOP`

最適化 opcode は、整数/浮動小数の fast path と、演算子再定義時の
通常 dispatch の両方を実装する。`Integer#+` などを再定義するテストが
あるため、`bop_redefined` 相当の状態を VM 側で管理する。

### 6.4 例外・制御

- catch handler テーブル（rescue / ensure）
- `EXCEPT` / `RESCUE` / `RAISEIF` / `JMPUW`
- `raise`, `begin/rescue/ensure`, `retry`, `redo`
- `return`, `break`, `next` の非局所制御
- DBG/LVAR を使った backtrace の file:line

### 6.5 パターンマッチ

- `MATCHERR`
- `deconstruct`, `deconstruct_keys`
- `__pat_values`, `__except`
- `NoMatchingPatternError`

## 7. コアライブラリ

最大の作業量。ハイブリッドで進める。

1. **Go 実装**
   - `BasicObject`, `Object`, `Kernel`, `Class`, `Module`
   - `NilClass`, `TrueClass`, `FalseClass`
   - `Integer`, `Float`, `String`, `Symbol`
   - `Array`, `Hash`, `Range`
   - `Proc`, `Method`
   - `Exception` 階層
2. **mruby の mrblib をロード**
   - mruby 4.0.0 の `mrblib` や pure-Ruby mrbgem を `go-mrbc` で
     コンパイルし、VM 起動時にロード。
   - C 実装が必要なメソッドは Go native で補う。
3. **公式テストで不足を埋める**
   - mruby 4.0.0 のテストを通しながらメソッドを追加。
   - ChibiRuby の `sig/*.rbs` をコア API のチェックリストとして使う。

最初から全 mrbgem を実装しない。適合コーパスに必要な範囲から始める。

## 8. GC 方針

- メモリ管理は Go GC。
- `ObjectSpace.each_object` のためにオブジェクトレジストリを持つ。
- `GC.start` は `runtime.GC()` にマップ。
- `GC.enable` / `GC.disable` はフラグを保持するが、Go GC 自体は停止できない。
  Ruby 可視の状態だけ再現する。
- `GC.stat` は `runtime.MemStats` から近似値を返す。
- `GC.interval_ratio` / `step_ratio` / `step_limit` は値を保存し、
  引数検証だけ mruby に合わせる。
- C mruby の GC 内部挙動を検査するテストは「対象外」または「近似」と明記する。
- finalizer は必要になったら `runtime.SetFinalizer` を限定的に使う。最初は使わない。

## 9. ホスト API

たたき台:

```go
type State struct{ /* ... */ }

type Options struct {
    Sandbox       *sandbox.Policy
    Deterministic bool
    LoadPaths     []string
}

func New(opts Options) *State
func (s *State) Close()

func (s *State) LoadBytecode(data []byte) (Value, error)
func (s *State) Execute(ctx context.Context, irep *bytecode.Irep) (Value, error)

func (s *State) DefineFunction(name string, fn Func)
func (s *State) DefineClass(name string, super *Class, configure func(*Class))
func (s *State) DefineModule(name string, configure func(*Module))
func (s *State) DefineConst(scope *Module, name string, value Value)

func (s *State) Send(ctx context.Context, recv Value, name string, args ...Value) (Value, error)
func (s *State) Raise(class *Class, message string)

type Func func(ctx context.Context, s *State, self Value, args []Value, block Value) (Value, error)
```

- Go の `error` を Ruby 例外に変換。
- `context.Context` でキャンセル。
- 値変換ヘルパー: `AsInt`, `AsFloat`, `AsString`, `AsBool`,
  `AsArray`, `AsHash`, `AsGoData`。
- ホストオブジェクトは `Data` で保持。
- 内部ポインタを公開しない。
- 1 `State` = 1 goroutine。

## 10. サンドボックス

- 命令数、実行時間、コール深さ、配列/ハッシュ/文字列サイズ、出力サイズ。
- `io` / `file` / `network` / `process` / `os` / `eval` / `load` / `require`
  の capability 許可リスト。
- 決定性モード（乱数シード、仮想時計、object_id、環境非依存）。
- ChibiRuby の `sandbox/` が参考になる。

## 11. コマンド

### `gomruby`

- `gomruby file.mrb` — バイトコードを実行
- `gomruby file.rb` — `go-mrbc` でコンパイルして実行
- `-e` / `-r` / `-I` / `-v` / `-c`

### `gomirb`

- 永続 `State`
- 入力ごとに `go-mrbc` でコンパイル
- 結果を `inspect`
- 複数行入力、履歴、`-r` preload、サンドボックス

## 12. テスト計画

### 差分実行

`cmd/vmcompat` を作る。

1. 同じソースを `gomrbc` と本家 `mrbc` でコンパイル
2. それぞれを C mruby と `go-mrbvm` で実行
3. stdout / stderr / exit status を比較

比較マトリクス:

| コンパイラ | オプション | 目的 |
| --- | --- | --- |
| `go-mrbc` | デフォルト | SEND 中心 |
| 本家 `mrbc` | デフォルト | 最適化 opcode |
| 本家 `mrbc` | `--no-optimize` | 最適化なし |

### テスト種別

- RITE ローダー: 本家 `mrbc` の出力を読めること
- opcode 単体: 119 opcode それぞれの最小 irep
- conformance: `testdata/conformance` を VM で実行
- mruby 公式テスト: `assert.rb` を前置して実行
- ファズ: RITE バイナリ破損、生成プログラム
- GC: `ObjectSpace`, `GC.start`, 大量確保、循環参照
- 例外: ensure 巻き戻し、backtrace
- ベンチマーク: C mruby 4.0.0 との比較

## 13. マイルストーン

| 段階 | 内容 | 完了条件 |
| --- | --- | --- |
| VM-0 | RITE ローダー、最小値、最小インタプリタ | `puts "hello"` が両コンパイラで動く |
| VM-1 | 式、ローカル、配列/ハッシュ、メソッド、ブロック、`yield` | 基本 conformance が通る |
| VM-2 | クラス、モジュール、継承、`super`、キーワード、クロージャ | クラス系 conformance が通る |
| VM-3 | 例外、ensure、retry/redo、backtrace、パターンマッチ | 例外系 conformance が通る |
| VM-4 | コアライブラリ、mrblib、公式テスト | conformance 全件 + コア subset |
| VM-5 | ホスト API、サンドボックス、`gomruby` | Go 関数を mruby から呼べる |
| VM-6 | `gomirb`、Fiber、Regexp/IO/JSON、デバッガ | REPL と opt-in 機能 |
| VM-7 | 性能、ベンチマーク、最適化 | C mruby との比較を公開 |

## 14. リスク

- コアライブラリの量が最大の作業。
- `ENTER` / 引数束縛（必須・オプション・rest・post・keyword・block・noblock）。
- `SUPER` / `ARGARY` / upvar とブロック・lambda の組み合わせ。
- Go GC と mruby の `ObjectSpace` / finalizer / GC パラメータの差。
- 最適化 opcode の fast path と再定義時 dispatch。
- Fiber の実装方式（goroutine か状態機械か）。
- Regexp（Go の RE2 は Ruby 互換ではない）。
- エンコーディング。
- `eval` / `load` / `require` の方針。
- 性能（interface boxing と dispatch）。

## 15. 直近の作業

1. VM-4 の実装
   - mrblib のロードと pure-Ruby メソッドの有効化
   - `Enumerable` / `Comparable` のモジュール化
   - `Enumerator` の最小実装
   - `String` / `Array` / `Hash` / `Integer` の残りメソッド
2. 公式テストの subset 実行
3. 例外の `backtrace` と `Exception#full_message`

## 16. 現在の状態

- リポジトリのスキャフォールド済み
- `go-mrbvm` モジュール作成済み
- `go-mrbc/bytecode` への依存と `DecodeBytecode` の入口あり
- 適合コーパス、サンプル、設計資料をコピー済み
- **VM-0 実装済み**
  - RITE 04.00 のロード
  - 最小の値・オブジェクトモデル
  - `STRING`, `MOVE`, `SSEND`, `RETURN`, `STOP` など
  - `Kernel#puts`
  - `go-mrbc` と本家 `mrbc` の `puts "hello"` が動作
  - `cmd/gomruby` から `.mrb` / `.rb` を実行可能
- **VM-1 実装済み（主要部分）**
  - ローカル変数、配列、ハッシュ、範囲、文字列、シンボル
  - メソッド、`ENTER` の引数束縛、ブロック、`yield`、upvar
  - `BLOCK` / `BLKPUSH` / `BLKCALL`、`Proc#call`
  - 最適化 opcode（`ADD`, `ADDI`, `ADDILV`, `SUB`, `EQ`, `LT` など）
  - 配列/ハッシュのパターンヘルパー
  - 次の conformance が両コンパイラで成功:
    `basic_hello`, `blocks`, `case_and_logic`, `collections`,
    `control_flow`, `methods`, `multiple_assignment`, `scalars`
- **VM-2 実装済み（主要部分）**
  - `CLASS` / `MODULE` / `EXEC` / `SCLASS` / `TCLASS`
  - クラス、モジュール、継承、`super`、`ARGARY`
  - クラスメソッド、`class << self`、クラス変数
  - キーワード引数、`**rest`、`KEY_P` / `KARG` / `KEYEND`
  - コアクラスの定数登録、`Class#===`、パターンヘルパー
  - 次の 20 conformance が両コンパイラで成功:
    `advanced_classes`, `alias_undef`, `assignment_targets`,
    `basic_hello`, `blocks`, `case_and_logic`, `classes`,
    `collections`, `control_flow`, `for_blocks`, `for_loops`,
    `keywords`, `methods`, `modern_syntax`, `multiple_assignment`,
    `operator_assignment`, `parameters`, `pattern_matching`,
    `scalars`, `splat`
- **VM-3 実装済み（主要部分）**
  - `raise` / `rescue` / `ensure` と例外クラス階層
  - catch handler、`EXCEPT` / `RESCUE` / `RAISEIF` / `MATCHERR`
  - `return` / `break` / `next` の ensure 巻き戻し
  - ブロックからの非局所 `return`、lambda の `return` / `break`
  - `retry` / `redo`、`JMPUW` の ensure 連携
  - 任意精度整数（bigint）と配列の Range スライス
  - 次の 29 conformance が両コンパイラで成功:
    `advanced_classes`, `alias_undef`, `assignment_targets`,
    `basic_hello`, `blocks`, `case_and_logic`, `classes`,
    `collections`, `control_flow`, `destructuring`, `ensure_control`,
    `exceptions`, `for_blocks`, `for_loops`, `keywords`, `lambda`,
    `lambda_super`, `methods`, `modern_syntax`, `multiple_assignment`,
    `operator_assignment`, `parameters`, `pattern_matching`,
    `patterns_extended`, `retry_redo`, `runtime_edges`, `scalars`,
    `semantic_edges`, `splat`
- **VM-4 進行中**
  - ネイティブの `Array` / `Hash` / `String` / `Integer` / `Range` /
    `Object` メソッドを拡充
  - `map` / `select` / `reject` / `find` / `inject` / `min` / `max` /
    `sort` / `group_by` / `sort_by` / `each_with_object` / `zip` /
    `flat_map`
  - `Hash#fetch` / `merge` / `dig` / `transform_*`、
    `String#split` / `sub` / `gsub` / `%` / `ljust` / `rjust` / `tr`、
    `Integer#times` / `upto` / `downto` / `step` / `div` / `divmod`、
    `Float#round` / `floor` / `ceil` / `nan?` / `infinite?` など
  - `Object#is_a?` / `send`、`Comparable` 相当の `<=>` 比較
  - `cmd/vmcompat` を追加（C mruby / go-mrbc / 本家 mrbc の差分実行）
  - ネイティブ `Enumerable` モジュールを追加し、ユーザー定義クラスの
    `each` に対して `map` / `select` / `inject` / `sort_by` などを提供
  - 非 strict ブロックの引数配列の自動分解（`|a, b|`）を実装
  - 最小 `Enumerator` を追加（`next` / `peek` / `rewind` / `size` /
    `with_index`）。ブロックなしの `each` / `map` / `times` などが
    Enumerator を返す
  - `core_library` / `core_library_extended` / `enumerable` /
    `enumerator` conformance を追加し、33 ケースが両コンパイラで成功
  - 次は mrblib のロードと `Enumerator`
