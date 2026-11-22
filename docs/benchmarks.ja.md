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
| `BenchmarkArithmeticLoop` | 10,000 回の加算ループ | 2,261,257 | 121,632 | 1,381 |
| `BenchmarkMethodCall` | 5,000 回のメソッド呼び出し | 1,713,307 | 123,465 | 1,404 |
| `BenchmarkArrayMap` | 100 要素の `map` × 100 回 | 1,327,416 | 420,252 | 1,913 |
| `BenchmarkHashWrite` | 5,000 回の Hash 書き込み | 1,819,371 | 2,120,267 | 1,448 |
| `BenchmarkStringAppend` | 2,000 回の文字列追加 | 708,151 | 2,377,431 | 7,380 |
| `BenchmarkRescue` | 2,000 回の raise/rescue | 1,004,059 | 826,615 | 17,387 |

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
| `BenchmarkArithmeticLoop` | 31,383 | 1,381 | 843,317 | 121,632 |
| `BenchmarkMethodCall` | 41,403 | 1,404 | 1,645,429 | 123,465 |
| `BenchmarkArrayMap` | 62,114 | 1,913 | 2,747,425 | 420,252 |
| `BenchmarkHashWrite` | 16,450 | 1,448 | 2,602,342 | 2,120,267 |
| `BenchmarkStringAppend` | 13,383 | 7,380 | 2,523,895 | 2,377,431 |
| `BenchmarkRescue` | 23,390 | 17,387 | 972,453 | 826,615 |

メソッド呼び出しの割り当ては約 29 分の 1、メモリ使用量は約 13 分の 1。
残る割り当ての大半は例外・バックトレースと `State` の初期化。

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
go-mrbvm  5 iterations  457.934625ms  10.92 ops/s
mruby     5 iterations  135.288667ms  36.96 ops/s
ratio     go-mrbvm/mruby = 3.38x
```

C mruby の計測にはプロセス起動時間が含まれるため、比率は目安。
負荷の高い環境では 3.6〜3.7x 程度になることもある。
今後の最適化対象:

- `Value` のボクシングとインターフェース割り当て
- 例外・バックトレースの割り当て（`BenchmarkRescue`）
- 文字列連結・`Hash` のバッファ成長
- メソッド探索のインラインキャッシュ
- `State` 初期化コスト

## 再現方法

```sh
go test ./mrbvm -run '^$' -bench . -benchtime=300ms -count=1
go run ./cmd/vmbench -n 5 -compare benchmark/mixed.rb
```
