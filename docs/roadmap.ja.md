# go-mrbvm ロードマップ

このドキュメントは `go-mrbvm` のフェーズ計画、直近スプリント、品質ゲートを
まとめたものである。実装の全体像は
[implementation-plan.ja.md](implementation-plan.ja.md)、設計は
[architecture.md](architecture.md) を参照。

最終更新: 2026-11-22

## 1. 現状

- VM-0〜VM-3 完了
- VM-4 進行中
  - ネイティブ `Array` / `Hash` / `String` / `Integer` / `Float` /
    `Range` / `Object`
  - `Enumerable` / `Enumerator` / `Comparable`
  - 特異クラス、`class_eval` / `alias_method` / `remove_method`
  - 例外、ensure、非局所制御、パターンマッチ
  - `cmd/vmcompat` / `cmd/mrbtest`
  - 共有 conformance **38 ケース**が `go-mrbc` と本家 `mrbc` の
    両方で成功
  - 公式テスト **20 ファイル**が成功（`testdata/official.txt`）
  - `Options.LoadMrblib` で 7 ファイルの mrblib subset を opt-in ロード
  - ホスト API（`DefineFunction` / `DefineClass` / `DefineModule` /
    `DefineConst` / `Send` / `Raise`）
  - サンドボックス（命令数・時間・深さ・出力・コレクションサイズ）と
    `sandbox.Policy` capability

## 2. 基本方針

1. **C mruby 4.0.0 を常に正とする**
   - `cmd/vmcompat` で stdout / exit status を比較する。
2. **両コンパイラを維持**
   - `go-mrbc` と本家 `mrbc` の両方で conformance を通す。
3. **ネイティブ実装を優先**
   - ホットパスは Go 実装。
   - mrblib は不足部分の補完として段階的に導入する。
4. **公式テストをマニフェスト化**
   - 通ったファイルを `go test` に固定し、回帰を防ぐ。
5. **各フェーズで品質ゲートを通す**
   - `gofmt` / `go vet` / `go test`
   - conformance 両コンパイラ
   - `vmcompat` 全件
   - 公式テスト subset

## 3. フェーズ計画

| フェーズ | 目標 | 主な作業 | 完了条件 |
| --- | --- | --- | --- |
| **VM-4a** コアライブラリ完成 | 公式 Array/Integer/String/Hash 系テストを通す | `Array#hash`、`freeze` / `frozen?`、`sort` の境界、`delete` / `shift` / `replace`、`initialize_copy`、`Float` / `RangeError` の整合 | `t/array.rb` が `KO: 0` / `Crash: 0`、conformance 40+ |
| **VM-4b** 公式テスト拡大（完了） | コア subset を一気に増やす | `integer.rb` / `float.rb` / `string.rb` / `hash.rb` / `range.rb` / `symbol.rb` / `exception.rb` を順に駆動 | 公式テスト 15 ファイル以上が成功 |
| **VM-4c** 例外・リフレクション（完了） | 実用的なエラー情報とメタプログラミング | `backtrace`（file:line）、`full_message`、`cause`、`$!`、`FrozenError`、`instance_variables`、`instance_variable_get/set`、`const_get/set`、`ancestors`、`methods` | `t/exception.rb` などが成功、backtrace が表示できる |
| **VM-5** ホスト API / サンドボックス（完了） | Go から mruby を安全に使う | `DefineFunction` / `DefineClass` / `DefineConst`、`Send`、値変換、Go error → Ruby 例外、命令数・時間・深さ・サイズ制限、capability | Go テストから Ruby 関数を呼び、Ruby から Go 関数を呼べる |
| **VM-6** `gomirb` / オプション機能（コア完了） | 対話環境と実用ライブラリ | REPL、複数行入力、履歴、JSON、IO、Time、Regexp（Go regexp 近似）、Fiber | `gomirb` で式評価、JSON/IO の基本動作 |
| **VM-7** 性能・GC（進行中） | C mruby と比較可能な性能 | dispatch 最適化、値表現、アロケーション削減、ベンチマーク、GC shim、`ObjectSpace` | ベンチマーク結果を公開、主要操作で許容範囲 |

## 4. 直近スプリント（2026-11-22 完了）

### 4.1 `t/array.rb` を完走させる（完了）

- `Array#hash`
  - 循環配列を含めて安全に計算する。
- `freeze` / `frozen?` / `FrozenError`
- `sort` / `sort!`
  - Float、bigint、NaN、非整数を返すブロック。
- `delete` / `delete_at` / `shift` / `pop` / `replace`
- `initialize_copy` / `dup` / `clone`
- `slice` / `first` / `last` の境界

**完了条件**: `mrbtest mruby/test/t/array.rb` が
`KO: 0` / `Crash: 0`。

### 4.2 公式テストマニフェスト（完了）

- `testdata/official.txt` を追加する。
- `cmd/mrbtest` に manifest 実行オプションを追加する。
- CI / `go test` で成功ファイルを固定する。
- 初期対象:
  - `nil.rb` / `true.rb` / `false.rb` / `comparable.rb`
  - 次に `integer.rb` / `float.rb` / `symbol.rb`

### 4.3 例外の実用化（完了）

- `Exception#backtrace` を irep の `Filename` / `Lines` から生成する。
- `Exception#full_message`
- `Exception#cause`
- `$!`（現在の例外）
- `FrozenError`
- `raise` の再送出・cause 連鎖

**完了条件**: 未捕捉例外で `file:line` が表示される。

### 4.4 リフレクション（完了）

- `Object#instance_variables`
- `Object#instance_variable_get` / `instance_variable_set`
- `Module#const_get` / `const_set`
- `Module#ancestors`
- `Object#methods` / `public_methods`
- `respond_to?` の private 引数

**完了条件**: 公式 `object.rb` / `module.rb` / `class.rb` の基本部分が通る。

### 4.5 mrblib の段階導入（完了）

- いきなり全部ロードしない。
- `Options.LoadMrblib` で opt-in にする。
- まず `10error.rb` / `compar.rb` / `enum.rb` のみ。
- ネイティブ実装との差分を conformance で比較する。
- 将来的に precompiled bytecode を `go:generate` + `go:embed` する。

**判断ポイント**: ネイティブ実装と mrblib のどちらを正とするか。

### 4.6 ホスト API の設計着手（完了）

- `DefineFunction`
- `DefineClass`
- `DefineConst`
- `Send`
- `Value` ↔ Go 型変換
- `context.Context` キャンセル

**完了条件**: Go のテストから Ruby メソッドを呼び、Ruby から Go 関数を
呼べる。

### 4.7 次のスプリント（VM-4b）

- 公式テストの拡大
  - `integer.rb` / `float.rb` / `string.rb` / `hash.rb` / `range.rb` /
    `symbol.rb` / `exception.rb`
  - 通ったファイルを `testdata/official.txt` に追加
- mrblib subset の拡張
  - `array.rb` / `hash.rb` / `string.rb` / `range.rb` を opt-in で追加
  - ネイティブ実装との差分を conformance で固定
- ホスト API の拡張
  - サンドボックスの命令数・時間・深さ制限
  - `Data` による Go オブジェクト保持
- 例外系の仕上げ
  - `backtrace` の file:line 精度向上
  - `full_message` の書式を C mruby に合わせる

### 4.8 VM-4b / VM-4c / VM-5 完了（2026-11-22）

- **VM-4b**
  - 公式テスト 20 ファイルを manifest 化
  - mrblib subset を 7 ファイルへ拡張
- **VM-4c**
  - backtrace（file:line）、full_message、cause、`$!`、FrozenError
  - instance_variables/get/set、const_get/set、ancestors、methods
  - 公式 `nameerror.rb` / `nomethoderror.rb` / exception クラス系テスト
- **VM-5**
  - `DefineFunction` / `DefineClass` / `DefineModule` / `DefineConst` /
    `Send` / `Raise`
  - 命令数・時間・深さ・出力・配列/ハッシュ/文字列サイズ制限
  - capability policy とテスト

公式 `t/exception.rb` も 50/50 で成功し、`$!` と cause 連鎖の
スコープ復元を確認済み。

### 4.9 VM-6 進行中（2026-11-22）

- 完了
  - `gomirb`（永続 State、複数行継続、`-r` / `-q`、非対話 stdin）
  - `Time`（`now` / `at` / 成分 / `strftime` / 加減算・比較）
  - `JSON`（`parse` / `generate` / `dump` / `pretty_generate`）
  - IO / File（`STDIN` / `STDOUT` / `STDERR`、path helper、size/delete）
  - Regexp / MatchData（フラグ、名前付きキャプチャ）
  - Fiber（`current` / `raise` / ensure）
  - `gomirb` の履歴永続化と補完候補
- 次
  - デバッガ連携
  - Regexp / Fiber / IO の gem 公式テスト拡大

### 4.10 VM-7 初期計測（2026-11-22）

- 完了
  - `mrbvm` の Go マイクロベンチマーク
  - `cmd/vmbench` と `benchmark/mixed.rb`
  - C mruby との比較（目安: 約 5x）
  - irep デコード結果の State キャッシュ
  - Hash のプリミティブキー索引（書き込み 148ms → 1.9ms / 5,000 件）
  - ブロック非生成フレームの register プール
  - GC shim（`GC.stat` / `count` / `enable?` / `start`）
  - C mruby 比 約 3.7x
- 次
  - 引数スライス割り当ての削減
  - `Value` のボクシング削減
  - メソッド探索キャッシュ
  - GC shim / `ObjectSpace`
  - 継続的なベンチマーク CI

詳細は [benchmarks.ja.md](benchmarks.ja.md) を参照。

## 5. 品質ゲート

各コミットで必ず実行する。

```sh
gofmt -l .
go vet ./...
go test ./...
go run ./cmd/vmcompat testdata/conformance/*/main.rb
go run ./cmd/mrbtest <成功している公式テスト>
```

追加ルール:

- conformance は **go-mrbc と本家 mrbc の両方**で確認する。
- 新しい公式テストが通ったら `mrbvm/official_test.go` に追加する。
- 仕様差分を見つけたら `testdata/conformance` にケース化する。
- C mruby と挙動が違う場合は、どちらを正とするかコメントに残す。

## 6. リスクと判断ポイント

| リスク | 対応 |
| --- | --- |
| mrblib が C プリミティブに依存 | ネイティブ実装を優先し、mrblib は段階導入 |
| 公式テストが GC / Fiber / Regexp に依存 | subset マニフェストで分離 |
| `Enumerator#next` の完全な遅延評価 | 現状は eager。必要になったら状態機械化 |
| 例外 backtrace の情報不足 | irep の debug info を整備 |
| 性能劣化 | VM-7 まで保留。まず正しさ |
| C API 互換 | mruby C API 全体ではなく、ホスト API として再設計 |

## 7. マイルストーン目安

- **VM-4a**: `t/array.rb` 完了（2026-11-22）、conformance 38
- **VM-4b**: 公式テスト 20 ファイル（達成）
- **VM-4c**: backtrace / reflection / exception 系テスト（達成、高度な cause 連鎖は継続）
- **VM-5**: ホスト API とサンドボックス（達成）
- **VM-6**: `gomirb` / Time / JSON / IO / Regexp / Fiber / 履歴・補完 完了、デバッガが残り
- **VM-7**: ベンチマーク基盤と初期最適化（進行中）

## 8. 関連ドキュメント

- [implementation-plan.ja.md](implementation-plan.ja.md) — 全体実装計画
- [architecture.md](architecture.md) — アーキテクチャ
- [bytecode-format.md](bytecode-format.md) — RITE 04.00
