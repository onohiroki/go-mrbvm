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

| ベンチマーク | 内容 | ns/op |
| --- | --- | ---: |
| `BenchmarkArithmeticLoop` | 10,000 回の加算ループ | 2,368,776 |
| `BenchmarkMethodCall` | 5,000 回のメソッド呼び出し | 1,881,376 |
| `BenchmarkArrayMap` | 100 要素の `map` × 100 回 | 1,684,021 |
| `BenchmarkHashWrite` | 5,000 回の Hash 書き込み | 1,855,222 |
| `BenchmarkStringAppend` | 2,000 回の文字列追加 | 742,416 |
| `BenchmarkRescue` | 2,000 回の raise/rescue | 962,683 |

### 改善実績: Hash 書き込み

キー索引を追加する前は 5,000 件の書き込みに約 148ms かかっていた。

| 変更 | ns/op |
| --- | ---: |
| キー索引なし | 約 148,000,000 |
| キー索引あり | 約 2,272,277 |

約 65〜84 倍の改善。プリミティブキー（Integer / String / Symbol など）は
Go の map で引き、それ以外は従来どおり線形比較にフォールバックする。

### 改善実績: フレーム register のプール

ブロックを生成しない irep のフレームでは register スライスを
`sync.Pool` で再利用する。

| ベンチマーク | 変更前 B/op | 変更後 B/op |
| --- | ---: | ---: |
| `BenchmarkMethodCall` | 2,642,037 | 1,645,429 |
| `BenchmarkArrayMap` | 4,423,853 | 2,747,425 |

メソッド呼び出しのメモリ使用量を約 38% 削減。

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
go-mrbvm  5 iterations  524.494084ms  9.53 ops/s
mruby     5 iterations  141.450959ms  35.35 ops/s
ratio     go-mrbvm/mruby = 3.71x
```

C mruby の計測にはプロセス起動時間が含まれるため、比率は目安。
今後の最適化対象:

- 引数スライスの割り当て削減
- `Value` のボクシングとインターフェース割り当て
- プロパティ/メソッド探索のキャッシュ
- `executeFrame` のクロージャ生成削減
- 例外・バックトレースの割り当て

## 再現方法

```sh
go test ./mrbvm -run '^$' -bench . -benchtime=300ms -count=1
go run ./cmd/vmbench -n 5 -compare benchmark/mixed.rb
```
