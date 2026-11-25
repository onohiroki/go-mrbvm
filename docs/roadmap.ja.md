# go-mrbvm ロードマップ

このドキュメントは `go-mrbvm` のフェーズ計画、直近スプリント、品質ゲートを
まとめたものである。実装の全体像は
[implementation-plan.ja.md](implementation-plan.ja.md)、設計は
[architecture.md](architecture.md) を参照。

最終更新: 2026-11-23

## 1. 現状

- VM-0〜VM-3 完了
- VM-4 進行中
  - ネイティブ `Array` / `Hash` / `String` / `Integer` / `Float` /
    `Range` / `Object`
  - `Enumerable` / `Enumerator` / `Comparable`
  - 特異クラス、`class_eval` / `alias_method` / `remove_method`
  - 例外、ensure、非局所制御、パターンマッチ
  - `cmd/vmcompat` / `cmd/mrbtest`
  - 共有 conformance **53 ケース**が `go-mrbc` と本家 `mrbc` の
    両方で成功（`go-mrbc/conformance` パッケージを共有）
  - `Integer` / `Float` の演算子メソッド層、`send` / alias /
    リフレクション、再定義時の opcode フォールバック
  - 公式テスト **42 ファイル**が成功（`testdata/official.txt`）
  - mrbgem テスト **18 スイート**が成功（`testdata/official-gems.txt`）
  - 言語意味論 L0–L4 完了（Symbol/Proc/Array/Hash/String/Enumerable/
    Enumerator/Kernel/メタプログラミング/Method/Rational/Complex/
    Regexp/Fiber/eval 基盤）
  - `Options.LoadMrblib` で 7 ファイルの mrblib subset を opt-in ロード
  - ホスト API（`DefineFunction` / `DefineClass` / `DefineModule` /
    `DefineConst` / `Send` / `Raise`）
  - サンドボックス（命令数・時間・深さ・出力・コレクションサイズ）と
    `sandbox.Policy` capability

## 2. 基本方針

1. **C mruby 4.0.0 を常に正とする**
   - `cmd/vmcompat` で stdout / exit status を比較する。
   - ただし既知の参照バグ（NaN の自己等価、負の即値、rescue 内 `next`）は
     CRuby と mruby 開発ツリーの挙動を正とし、共有コーパスには入れない。
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
名前が重なる場合はネイティブ実装を正とし、mrblib は不足分を補う。
たとえば mrblib の `string.rb` は String パターン専用の `sub` / `gsub`
を定義するため、ロード後に Regexp 対応のネイティブ実装を再登録する。

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
  - 公式テスト 20 ファイルを manifest 化（現在 31）
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
  - ブロック非生成フレームの register / 引数 / 環境プール
  - native 呼び出しの引数コピー削減とブロック引数のスタック渡し
  - `return` の `unwindSignal` 割り当て削減
  - Hash 索引の開放アドレス化（`Hash#delete` 後の重複 entry も修正）
  - 文字列 `<<` の追記バッファと `String#dup` 修正
  - バックトレース整形のフレームキャッシュ
  - `Object` とペイロード（String/Array/Hash/Block/Exception）の同一アロケーション
  - 行番号テーブル・制限チェック・private 検査・defer のホットループ最適化
  - 1 引数 SEND の演算子 ID 事前計算と整数専用高速パス
  - ジャンプ先の事前解決と行番号更新の抑制
  - 公式 `mrbc` の `OP_ADD` 系命令を整数専用に高速化
  - `vmbench -mrbc` で公式バイトコードとの比較が可能に
  - バックトレース行を呼び出し時・例外時だけ計算
  - GC shim（`GC.stat` / `count` / `enable?` / `start`）
  - MethodCall の割り当て 41,403 → 1,399 / 回
  - C mruby 比 約 1.04x（公式 mrbc）/ 約 1.33x（go-mrbc）
- 次
  - `Value` のボクシング削減
  - `State` 初期化コスト
  - メソッド探索キャッシュ
  - GC shim / `ObjectSpace`
  - 継続的なベンチマーク CI

詳細は [benchmarks.ja.md](benchmarks.ja.md) を参照。

### 4.11 数値・文字列の意味論強化（2026-11-22）

- 整数演算のオーバーフローを bigint に昇格（`+ - * /`、単項 `-`、`succ` など）
- Float の `to_s` / `inspect` を mruby と同じ 15 桁・`0.0` 表記・`Infinity` に統一
- Hash の `0.0` / `-0.0` を同一キーとして扱う
- `Integer#to_s(base)` / `lcm`、`Float#divmod` / `fdiv`、
  `Float#round/floor/ceil(ndigits)`、`clamp` の引数検査を追加
- 整数のビット演算 `&` / `|` / `^` / `~` を追加
- `String#each_char` / `each_byte` / `casecmp` / `casecmp?`、
  `split` の limit、文字集合の `^` 否定を追加
- `sprintf` / `%` を書き直し（整数精度・符号・幅・`0` 埋め、基数変換、
  Float の丸め、`Inf`/`NaN`、名前付き参照）
- conformance を 45 ケースに拡大（`integer_overflow` / `float_format` /
  `numeric_edges` / `format_string` / `string_extra`）

### 4.12 公式テスト互換性の強化（2026-11-22）

優先順に Hash / Range / Module・Class / String を強化した。

- **Hash**
  - オブジェクトキーで Ruby の `hash` / `eql?` を呼ぶ
  - `compare_by_identity` / `rehash` / `default_proc`
  - `store` / `clear` / `shift` / `replace` / `assoc` / `rassoc`
  - String キーの複製と freeze、Hash 反復の `[key, value]` ペア渡し
- **Range**
  - 文字列・任意オブジェクトの `each`（`succ`）、endless range の `break`
  - Float 終端付き `to_a`、beginless/endless の例外、NaN 境界の検査
  - `Range.new` / `member?` / 汎用 `to_s` / `inspect` / `eql?`
- **Module / Class**
  - `Module.new.class` などモジュールとクラスの区別
  - `include` / `prepend` のフック（`append_features` / `prepended`）と複数指定
  - `public` / `private` / `protected` の可視性モード、`module_function`
  - `define_method` / `singleton_class` / `extend` / `remove_const` /
    `undef_method` / `*_constant`
- **String**
  - `initialize` / `initialize_copy` / 破壊的メソッド / `each_line` /
    `setbyte` / `byterindex` / `bytesplice` / `[]=`
  - `to_i` / `to_f` の mruby 互換スキャナ、`index` / `rindex` の offset
  - String サブクラスの文字列としての等価性・表示
  - `String#+` / `==` の再定義を OP_ADD / OP_EQ でも尊重

公式テストの通過状況（このセッションの計測）:

| ファイル | OK / Total | 備考 |
| --- | ---: | --- |
| `hash.rb` | 57 / 57 | 全通過（再入ガード・pair 保持を含む） |
| `range.rb` | 24 / 26 | 残りは mruby GC テスト用ヘルパー |
| `class.rb` | 49 / 49 | 全通過 |
| `module.rb` | 86 / 86 | 全通過 |
| `string.rb` | 104 / 104 | 全通過（UTF-8 文字単位モード） |

追加で以下を解決した。

- メソッド探索を実際の祖先チェーン（prepend / self / include / superclass）
  に変更し、`super` が定義元モジュールの次から探索するようにした
- 可視性モードをクラス本体・`class_eval`・`Module.new` ブロックの
  スコープに限定し、メソッド内や別クラスへの呼び出しへ漏れないようにした
- private/protected クラスメソッド、`public_class_method`、
  `extend_object` / `extended`、定数パス探索
- Hash の frozen 検査と `reject!` / `select!` の nil 返却
- String の `getbyte` / `byteslice`、補間での `to_s` 呼び出しと例外伝播、
  `sub` / `gsub` の `\\`` / `\\'` と後方参照、空パターン
- Hash の Array/Hash 等価性、`compare_by_identity`、`rehash`、`assoc`、
  ライブなイテレーションと frozen 検査
- Class の再オープン時 superclass 検査、`Class.new` の `inherited` 順、
  入れ子クラスの名前と定数パス、`Module#dup`、`undef_method`
- `attr_*` の返り値・String 名・可視性、`private_class_method`、
  `module_function` のトグル、protected クラスメソッド
- 祖先チェーンの重複（superclass をまたぐ prepend）と include/prepend の
  先勝ち、クラス変数の所属解決と frozen 検査
- `class_eval` / lambda の可視性スコープ、`method_missing` の可視性無視、
  Proc#call へのブロック引き渡し
- Unicode の完全な upper/lower/title 変換表（ß→SS、ﬁ→FI、ǳ→ǲ、İ→i̇、
  ΐ→Ϊ́、Georgian Mkhedruli など）と不正 UTF-8 の ArgumentError
- レキシカル cref に基づく定数探索（クラス本体・`class << self`・
  `class_eval` ブロック・lambda）、クラス変数のレキシカル解決
- `defined?` を VM の直前の定数/変数参照から判定し、未定義・private
  定数でも評価せずに nil を返す
- `String#[]` / `[]=` の String インデックス、`bytesplice` の全引数形、
  Range#to_a の Float 上限（2^63 境界）
- `extend` のクラス/特異クラス/モジュール解決、継承メソッドの可視性変更
- 公式テスト実行用の C テスト gem 互換スタブ（`Mrbtest`・
  `TestNotImplement`）を mrbtest の prelude に追加

### 4.13 共有コーパスと演算子メソッド層（2026-11-22）

- `go-mrbc` に `conformance` パッケージを追加し、53 ケースを
  `embed.FS` で共有。`go-mrbvm` はローカルコピーを廃止し、
  `mrbvm/conformance_test.go` が全ケースを動的に実行する。
- `cmd/vmcompat -corpus` で共有コーパス全体を C mruby /
  go-mrbc / 本家 mrbc の三者で比較できるようにした。
- `Integer` / `Float` の
  `+ - * / % ** << >> & | ^ ~ -@` を実メソッドとして登録。
  `alias_method` / `send` / `method_defined?` / `respond_to?` が
  組み込み演算子を参照できる。
- 演算子が再定義されたら `bopRedefined` を立て、`OP_ADD` 系、
  `OP_ADDI` / `OP_SUBI`、`OP_ADDILV` / `OP_SUBILV`、および
  `SEND` の演算子高速パスを `s.call` にフォールバック。
  `a == a` の同一性ショートカットも再定義を尊重する。
- `Integer#<<` / `>>` の負のシフト、`FloatDomainError` 定数、
  `Mrbtest::FLOAT_TOLERANCE`、複数名の `remove_method` を追加。
- `TestOperatorRedefinition` で CRuby / 開発ツリーの再定義挙動を固定。
  共有 `operator_methods` は 4.0.0 と CRuby の両方で同じ結果になる
  alias / send / reflection だけを検証する。
- `vmbench`（mixed.rb、20 iterations）:
  go-mrbc bytecode 1.256s → 1.216s、公式 mrbc bytecode
  1.307s → 1.279s。演算子メソッド層による退行はない。
- `integer.rb` / `float.rb` は全項目通過。Float 比較の精度、整数の丸め、
  `__ensure`、NaN の同一性も解消した。

### 4.14 コア言語ギャップの解消（2026-11-22）

公式テストを 22 ファイルから 35 ファイルへ拡大した。追加で通したもの:

`argumenterror` / `autoload` / `basicobject` / `bs_block` / `ensure` /
`enumerable` / `hash` / `integer` / `float` / `methods` / `numeric` /
`proc` / `shareable_constant` / `string` / `symbol`

- `Proc#[]` / `arity` / `==` / `hash`、`&obj` の `to_proc` 変換
- `Symbol#to_sym`、`BasicObject#defined?`
- `Enumerable#grep`、`detect`/`find` の ifnone、`include?` の同一性比較
- `method_added` / `singleton_method_added`、存在しない `undef` の NameError
- lambda / method を跨ぐ `return` と ensure、nested block の post 引数
- `defined?` がメソッド呼び出し・`yield`・`super` を評価しないこと
- 定数代入後の `defined?` が `"assignment"` を返すこと
- Int/Float の厳密比較、Integer の丸め、Float の `to_i`/`round` 境界
- `Numeric#step`（nil/無限大の limit・step、混合 Int/Float、break）
- Numeric サブクラスの `to_f` 変換（method_missing 経由を含む）
- `Module#autoload` / `autoload?`、`const_get` / `const_defined?` /
  `defined?` / `remove_const` の pending 解決、superclass・include・
  frozen モジュール、読み込み中の定数不可視、require 失敗時の復元
- Hash の再入ガード（`hashData.version`）: `hash` / `eql?` / `==` /
  `inspect` コールバック中の構造変更を `RuntimeError` にし、
  `merge` / `__except` では pair をスナップショットして保持
- `Hash#inspect` がキー・値の Ruby メソッドを dispatch するように変更
- String を **UTF-8 文字単位** に正式化（mruby デフォルトの ASCII-8BIT とは
  異なるプロジェクト判断）。`length` / `[]` / `index` / `inspect` / Regexp の
  位置 / sprintf の幅を文字単位に統一し、不正 byte は 1 byte = 1 文字として
  元の byte を保持。byte API は `bytesize` / `byteslice` / `bytesplice` /
  `each_byte` / `byteindex` / `byterindex` に分離
- 実行時 NaN に一意の payload を与え、`equal?` / `object_id` を区別

残る失敗は次のカテゴリに限定している:

- GC 内部
- `env` / `gc` / `range` の GC テスト、`sysfail` / `vformat` の C gem ヘルパ
- `codegen` / `syntax` の 4.0.0 以降の構文

conformance は 53 ケースに拡大（`go-mrbc` の `conformance` パッケージで共有）。除外しているのは mruby GC 内部と C テスト gem ヘルパ、4.0.0 以降の構文のみ。

### 4.15 言語意味論 L0–L4（2026-11-23）

公式コアテストを 35 から **42 ファイル**、mrbgem テストを **34 スイート**
まで拡大した。L0–L4 の順に、各マイルストーンをコミットで固定している。

- **L0** 通過済みコアテストのマニフェスト化
- **L1** `Symbol` 拡張、`Proc` 拡張（`source_location` / `inspect` /
  `lambda?` / `arity` / `curry` / `<<` / `>>`、`break` の
  `LocalJumpError` 化）
- **L2** `Array` / `Hash` / `String` / `Enumerable` 拡張、
  遅延 `Enumerator`（`next` / `peek` / `feed` / `rewind` /
  `Enumerator.new` / `produce`）
- **L3** `Kernel` / メタプログラミング / `Method` / `UnboundMethod`
  （`caller`、`Binding` なしのフレーム情報、`define_method` の
  Method 受け取り、`super` のモジュール位置追跡）
- **L4**
  - `Rational`（`big.Rat`、正確な比較・演算・丸め）
  - `Complex`（部品クラスを保持、scaled division、`hash` / `eql?`、
    `Math` モジュール）
  - `Regexp` / `MatchData`（`dlclark/regexp2`、オプション畳み込み、
    `escape` / `union`、`$~` / `$1`–`$9` / `` $& `` / `` $` `` / `$'` /
    `$+`、`String` / `Symbol` の regexp メソッド、Onigmo 構文の翻訳層）
  - `Fiber`（goroutine + 協調スケジューラ、`transfer` / `raise` /
    `storage`、per-fiber VM スタック）
  - `eval` / `Binding` / `load` / `require`（`Options.Compiler` 注入。
    VM コアはコンパイラをリンクしない）
- **L5 相当（C gem のネイティブ移植）**
  - `Set`（挿入順 Hash 表、再入時の比較、集合演算）
  - `Struct`（クラス生成、位置/keyword 初期化、値等価、パターンマッチ）
  - `Data`（不変メンバー、`with`、`deconstruct_keys`、freeze）

通過状況（2026-11-23 時点）:

| スイート | OK / Total | 備考 |
| --- | ---: | --- |
| `mruby-proc-ext` | 10 / 14 | 残り 4 件は C ヘルパ依存 |
| `mruby-enum-ext` | 44 / 44 | ObjectSpace テストは skip |
| `mruby-enumerator` | 52 / 52 | 全通過 |
| `mruby-method` | 39 / 40 | ObjectSpace テスト 1 件 skip |
| `mruby-rational` | 150 / 150 | Complex 連携を含む |
| `mruby-complex` | 82 / 82 | Math 連携を含む |
| `mruby-set/set.rb` | 49 / 50 | ObjectSpace 1 件 skip |
| `mruby-struct/struct.rb` | 47 / 50 | ObjectSpace 3 件 skip |
| `mruby-data/data.rb` | 19 / 21 | ObjectSpace 2 件 skip |
| `mruby-regexp/regexp.rb` | 45 / 45 | 全通過 |
| `mruby-regexp/string_regexp.rb` | 73 / 74 | C engine の step/stack limit 1 件 skip |
| `mruby-regexp/string_index.rb` | 40 / 43 | 上記 1 件 + ObjectSpace 2 件 skip |
| `mruby-regexp/symbol_regexp.rb` | 11 / 11 | 全通過 |
| `mruby-regexp/match_data.rb` | 55 / 55 | 全通過 |
| `mruby-regexp/capture_slots.rb` | 5 / 5 | 全通過 |
| `mruby-regexp/end_anchor_window.rb` | 10 / 10 | 全通過 |
| `mruby-regexp/linebreak_escape.rb` | 4 / 4 | `\R` 翻訳を含む |
| `mruby-regexp/required_literal.rb` | 7 / 7 | 全通過 |
| `mruby-fiber/fiber.rb` | 24 / 25 | GC stress 1 件 skip |
| `mruby-fiber/fiber2.rb` | 6 / 6 | 全通過 |
| `mruby-eval/eval.rb` | 45 / 48 | C テストヘルパ 1 件 skip、stress 2 件は未実行 |
| `mruby-eval/binding.rb` | 14 / 14 | 全通過 |

`mruby-regexp` の翻訳層は、Ruby の `^` / `$`、Onigmo の `{,n}` と
possessive quantifier、`\R` / `\h` / `\H`、`\u{...}`、POSIX bracket、
名前付きグループの番号規則（同名グループの分離、名前付きがある場合の
無名グループの非捕捉化）を `regexp2` の構文に写す。byte-indexed 文字列は
1 byte = 1 rune に写して検索する。

### 4.16 残作業（2026-11-23 時点）

- `mruby-regexp` の次の suite は `regexp2` が表現できない構文、または
  C engine の内部状態に依存するため除外している:
  - `regexp_call`（`\g<name>` のサブルーチン呼び出し・再帰）
  - `regexp_syntax`（absent repeater `(?~...)`、conditional、`\K`、
    class intersection、nested class、C の stack/step limit）
  - `regexp_utf8`（invalid UTF-8 の byte 単位マッチ、`\x` byte と
    character の区別、C engine の byte-mode）
  - `backref_scope`（呼び出しスコープごとの `$~` 保存と C テストヘルパ）
  - `ascii_case` / `ascii_ctype`（ASCII ctype build 専用）
  - `unicode_case` / `unicode_ctype` / `unicode_property`（Unicode の
    property table と emoji property。`regexp2` の一般カテゴリだけでは
    POSIX bracket の全メンバーを再現できない）
- `mruby-eval` は `super` / `yield` / `return` の呼び出しフレーム転送、
  可視性の伝播、名前・pool の長さ上限、`_1` の扱い、binding をまたぐ
  eval ローカルと proc の共有まで対応した。定数キャッシュの C テスト
  ヘルパだけを skip する。
- GC 内部・`ObjectSpace`・C テストヘルパに依存するテストは引き続き除外。
- VM-7 の性能・GC 作業は継続。

## 5. 品質ゲート

各コミットで必ず実行する。

```sh
gofmt -l .
go vet ./...
go test ./...
go run ./cmd/vmcompat -corpus
go run ./cmd/mrbtest -manifest testdata/official.txt
go run ./cmd/mrbtest -manifest testdata/official-gems.txt
```

追加ルール:

- conformance は **go-mrbc と本家 mrbc の両方**で確認する。
- 新しい公式テストが通ったら `mrbvm/official_test.go` に追加する。
- 仕様差分を見つけたら `go-mrbc/conformance/testdata/conformance` にケース化する。
- C mruby と挙動が違う場合は、どちらを正とするかコメントに残す。

## 6. リスクと判断ポイント

| リスク | 対応 |
| --- | --- |
| mrblib が C プリミティブに依存 | ネイティブ実装を優先し、mrblib は段階導入 |
| 公式テストが GC / Fiber / Regexp に依存 | subset マニフェストで分離 |
| `Enumerator#next` の完全な遅延評価 | goroutine による suspend/resume で実装済み |
| 例外 backtrace の情報不足 | irep の debug info を整備 |
| 性能劣化 | VM-7 まで保留。まず正しさ |
| C API 互換 | mruby C API 全体ではなく、ホスト API として再設計 |

## 7. マイルストーン目安

- **VM-4a**: `t/array.rb` 完了（2026-11-22）、conformance 38
- **VM-4b**: 公式テスト 42 ファイル、mrbgem 34 スイート（達成）
- **VM-4c**: backtrace / reflection / exception 系テスト（達成、高度な cause 連鎖は継続）
- **VM-5**: ホスト API とサンドボックス（達成）
- **VM-6**: `gomirb` / Time / JSON / IO / Regexp / Fiber / 履歴・補完 / 言語意味論 L0–L4 完了、デバッガと残りの gem suite が残り
- **VM-7**: ベンチマーク基盤と初期最適化（進行中）

## 8. 関連ドキュメント

- [implementation-plan.ja.md](implementation-plan.ja.md) — 全体実装計画
- [architecture.md](architecture.md) — アーキテクチャ
- [bytecode-format.md](bytecode-format.md) — RITE 04.00
