# go-mrbvm 実装計画（概要）

英語の詳細版は [go-mrbvm-plan.md](go-mrbvm-plan.md) を参照。
このファイルは日本語での要点整理である。

## 目的

- `github.com/onohiroki/go-mrbvm` として、pure Go の mruby 4.0.0 VM を実装する。
- `go-mrbc` が生成する RITE 04.00 と、本家 `mrbc` が生成する RITE 04.00 を実行する。
- Go ツールのマクロ言語として埋め込めるホスト API を提供する。
- `gomruby`（mruby 相当）と `gomirb`（mirb 相当）を提供する。

## リポジトリ

```text
go-mrbc   … コンパイラ、RITE writer/reader、bytecode、適合コーパス
go-mrbvm  … VM、オブジェクトモデル、GC、ホスト API、サンドボックス、CLI
```

依存方向は `go-mrbvm → go-mrbc/bytecode` の一方向。
コンパイラは VM に依存しない。

## 共有契約

- `github.com/onohiroki/go-mrbc/bytecode`
- `ReadBinary`、`Irep`、`PoolEntry`、`CatchHandler`、`Instruction`、`Decode`
- RITE 04.00、LVAR/DBG
- 開発中は `go.work`、リリースは `go-mrbc` のタグに依存
- 将来的に `bytecode` を独立モジュールへ切り出し可能

## VM の構成

- 値: 最初はタグ付き構造体（kind + uint64 + Object ポインタ）。
  Go GC から見える形を優先し、NaN boxing は後回し。
- オブジェクト: class、ivars、String/Array/Hash/Range/Proc/Exception/Fiber。
- 実行: irep、コールフレーム、レジスタ、upvar、block、catch handler。
- 全 opcode を実装（`SEND` 系、`ENTER`、`BLOCK`、`SUPER`、`ARGARY`、
  `GETUPVAR`、`JMPUW`、`MATCHERR` など）。
- 例外: `raise` / `rescue` / `ensure` / `retry` / `redo`、backtrace。
- GC: まず Go GC を使い、`ObjectSpace` やメモリ制限のために
  オブジェクトレジストリを用意。必要なら後で独自 GC。
- Fiber: まずは後回し。必要になったら goroutine ベースか
  インタプリタの状態機械で実装。
- Regexp: Go の RE2 は Ruby 互換ではないため、
  opt-in 機能として扱い、制限を明記する。
- BigInt: `math/big`。Rational/Complex は opt-in。

## コアライブラリ

mruby のコアは C と mrblib の混在なので、ハイブリッドで進める。

1. Go で primitive/semantic なクラスを実装
   （Object、Kernel、Integer、Float、String、Array、Hash、Proc、Exception など）
2. mruby の mrblib を `go-mrbc` でコンパイルしてロード
3. 速度やホストアクセスが必要な部分だけ Go native
4. mruby 公式テストをチェックリストにする

ChibiRuby の `sig/*.rbs` はコア API の棚卸しに使える。

## ホスト API

たたき台:

```go
type State struct{ /* ... */ }

func New(opts Options) *State
func (s *State) LoadBytecode(data []byte) (Value, error)
func (s *State) Execute(ctx context.Context, irep *bytecode.Irep) (Value, error)
func (s *State) DefineFunction(name string, fn Func)
func (s *State) DefineClass(name string, super *Class, configure func(*Class))
func (s *State) Send(ctx context.Context, recv Value, name string, args ...Value) (Value, error)
func (s *State) Raise(class *Class, message string)

type Func func(ctx context.Context, s *State, self Value, args []Value, block Value) (Value, error)
```

- Go の `error` を mruby 例外に変換
- `context.Context` でキャンセル
- 値変換ヘルパー
- ホストオブジェクトは `Data` で保持
- 1 State = 1 goroutine。複数 goroutine は State を分ける

## サンドボックス

- 命令数、実行時間、コール深さ、配列/ハッシュ/文字列サイズ、出力サイズ
- `io` / `file` / `network` / `process` / `os` / `eval` / `load` / `require`
  の capability 許可リスト
- 決定性モード（乱数シード、仮想時計、object_id、環境非依存）
- ChibiRuby の `sandbox/` が参考になる

## テスト

- `go-mrbc` の適合コーパスを共有
- `cmd/vmcompat`: `gomrbc` でコンパイル → C mruby と `go-mrbvm` で実行比較
- opcode 単体テスト
- RITE ローダーのファズテスト
- ランダム差分テスト
- GC / Fiber / サンドボックスのストレステスト
- C mruby 4.0.0 とのベンチマーク

## マイルストーン

- VM-0: RITE ローダー、最小値、`puts "hello"`
- VM-1: 式、ローカル、配列/ハッシュ、メソッド、ブロック、`yield`
- VM-2: クラス、モジュール、継承、`super`、キーワード引数、クロージャ
- VM-3: 例外、ensure、retry/redo、backtrace、パターンマッチ
- VM-4: コアライブラリ、mrblib、適合コーパス
- VM-5: ホスト API、サンドボックス、`gomruby`、`gomirb`
- VM-6: Fiber、デバッガ、Regexp/IO/JSON、性能

## ChibiRuby から学ぶこと

- pure managed language でも mruby 4.0 の公式テストを通せる
- 全 opcode の実装が必須
- ホスト API の使い勝手が重要
  （DefineClass/DefineMethod/Send/Raise/値変換）
- Regexp/IO/JSON は opt-in にしてデフォルトの攻撃面を小さくする
- Fiber と async は後付けにしない
- デバッガは大きな価値がある
- native mruby とのベンチマークを公開する
- RBS シグネチャはコア API のチェックリストになる

ただし C# の JIT/GC/async をそのまま Go に持ち込まない。
ChibiRuby のコンパイラは native 依存だが、`go-mrbvm` は `go-mrbc` を使って
pure Go を維持する。ChibiRuby は MIT ライセンスなので、コードを移植する場合は
ライセンス表記と attribution を保持する。

## リスク

- コアライブラリの量が大きい
- Regexp の Ruby 互換性
- Fiber の実装方式
- GC / ObjectSpace / メモリ制限
- Go の interface boxing による性能
- エンコーディング
- `eval` / `load` / `require` の方針
- mrbgems（C 拡張は対象外）

## 最初の完了条件

- `gomruby` が `go-mrbc` の適合コーパスを実行できる
- mruby 4.0.0 のコアテスト subset を通る
- Go 関数・クラスを登録して mruby から呼べる
- `gomirb` が複数行入力と preload に対応
- 命令数・時間・深さのサンドボックス制限が動く
- backtrace に file:line が出る
- cgo 不要
- ベンチマークが公開されている
