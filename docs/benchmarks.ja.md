# go-mrbvm ベンチマーク

最終更新: 2026-11-22

## 環境

- CPU: Apple M4
- Go: 1.27.4 (darwin/arm64)
- 参照 mruby: 4.0.0

数値は実行環境・負荷・ウォームアップで変動する。絶対値ではなく、
変更前後の比較と C mruby との目安として扱う。

## マイクロベンチマーク

```sh
go test ./mrbvm -run '^$' -bench . -benchtime=300ms -count=1
```

| ベンチマーク | 内容 | ns/op | B/op | allocs/op |
| --- | --- | ---: | ---: | ---: |
| `BenchmarkArithmeticLoop` | 10,000 回の加算ループ | 2,246,708 | 121,744 | 1,381 |
| `BenchmarkMethodCall` | 5,000 回のメソッド呼び出し | 1,698,154 | 123,656 | 1,404 |
| `BenchmarkArrayMap` | 100 要素の `map` × 100 回 | 1,305,805 | 420,410 | 1,913 |
| `BenchmarkHashWrite` | 5,000 回の Hash 書き込み | 1,494,907 | 1,464,593 | 1,423 |
| `BenchmarkStringAppend` | 2,000 回の文字列追加 | 528,217 | 319,201 | 5,392 |
| `BenchmarkRescue` | 2,000 回の raise/rescue | 840,099 | 826,338 | 13,387 |

`ns/op` はマシン負荷で大きく変動するため参考値。`B/op` と `allocs/op` は
ほぼ安定しており、最適化の効果はこちらで判断しやすい。

### 改善実績: Hash 書き込み

キー索引を追加する前は 5,000 件の書き込みに約 148ms かかっていた。

| 変更 | ns/op |
| --- | ---: |
| キー索引なし | 約 148,000,000 |
| キー索引あり | 約 2,272,277 |

約 65〜84 倍の改善。プリミティブキー（Integer / String / Symbol など）は
Go の map で引き、それ以外は従来どおり線形比較にフォールバックする。

### 改善実績: 呼び出しごとの割り当て削減

`sync.Pool` にスライスを入れるとインターフェースへのボックス化で毎回
割り当てが発生するため、`State` 内の自由リスト（`[][]Value` と
`[]*environment`）で register・引数・フレーム環境を再利用する。
ブロックを生成しない irep（`BLOCK` / `LAMBDA` を含まない）だけが対象。

あわせて以下を削減した。

- native メソッド呼び出しで引数スライスをコピーしない
  （保持する `p` やホスト関数、Enumerator だけがコピーする）
- `callBlock1` / `callBlock2` でブロック引数をスタック上のバッファで渡す
- `return` 時に例外ハンドラがなければ `unwindSignal` を割り当てない
- フレームの `defer` クロージャを通常の関数呼び出しに置き換える
- `callFrame` をスライス内の値として保持する

| ベンチマーク | 変更前 allocs/op | 変更後 allocs/op | 変更前 B/op | 変更後 B/op |
| --- | ---: | ---: | ---: | ---: |
| `BenchmarkArithmeticLoop` | 31,383 | 1,381 | 843,317 | 121,744 |
| `BenchmarkMethodCall` | 41,403 | 1,404 | 1,645,429 | 123,656 |
| `BenchmarkArrayMap` | 62,114 | 1,913 | 2,747,425 | 420,410 |
| `BenchmarkHashWrite` | 16,450 | 1,423 | 2,602,342 | 1,464,593 |
| `BenchmarkStringAppend` | 13,383 | 5,392 | 2,523,895 | 319,201 |
| `BenchmarkRescue` | 23,390 | 13,387 | 972,453 | 826,338 |

メソッド呼び出しの割り当ては約 29 分の 1、メモリ使用量は約 13 分の 1。
残る割り当ての大半は例外オブジェクト、文字列リテラル、`State` の初期化。

### 改善実績: Hash 索引の開放アドレス化

プリミティブキーの索引を `map[hashKey]int`（キー 40 バイト）から
2 の冪サイズの `[]int` と線形探索へ変更。索引が対象とする entry 数を
記録し、長さが変わったら再構築する。これにより `Hash#delete` 後の
再代入で重複 entry ができる既存バグも解消した。

### 改善実績: 文字列の追記バッファ

`String#<<` は毎回 Go 文字列を連結しており、n バイトの文字列を作ると
O(n²) バイトを割り当てていた。追記用の `[]byte` バッファに貯め、
読み出し時にだけ文字列化する。フィールド名を `raw` に変えて直接参照を
コンパイルエラーにし、`str` / `set` / `appendString` 経由に統一した。

### 改善実績: バックトレース整形のキャッシュ

同じ行から繰り返し raise される場合に備え、フレームごとに整形済みの
`file:line` を保持する。`BenchmarkRescue` は 17,387 → 13,387 allocs/op。

## C mruby との比較

```sh
go run ./cmd/vmbench -n 5 -compare benchmark/mixed.rb
```

`benchmark/mixed.rb` は以下を組み合わせる。

- 200,000 回の加算ループ
- `fib(22)` の再帰
- 20,000 件の Hash 書き込み
- 1,000 要素の `map` × 200 回
- 5,000 回の文字列追加

結果例:

```
go-mrbvm  10 iterations  1.030637833s  9.70 ops/s
mruby     10 iterations  306.453208ms  32.63 ops/s
ratio     go-mrbvm/mruby = 3.36x
```

C mruby の計測にはプロセス起動時間が含まれるため、比率は目安。
負荷の高い環境では 3.6〜3.7x 程度になることもある。
今後の最適化対象:

- `Value` のボクシングとインターフェース割り当て
- 例外オブジェクト（`Object` と `exceptionData` の二重割り当て）
- 文字列リテラル読み込み（`StringValue` の二重割り当て）
- メソッド探索のインラインキャッシュ
- `State` 初期化コスト

## 再現方法

```sh
go test ./mrbvm -run '^$' -bench . -benchtime=300ms -count=1
go run ./cmd/vmbench -n 5 -compare benchmark/mixed.rb
```
