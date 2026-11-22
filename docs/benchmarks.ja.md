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
| `BenchmarkArithmeticLoop` | 10,000 回の加算ループ | 863,512 | 121,697 | 1,376 |
| `BenchmarkMethodCall` | 5,000 回のメソッド呼び出し | 951,114 | 123,432 | 1,397 |
| `BenchmarkArrayMap` | 100 要素の `map` × 100 回 | 903,804 | 421,086 | 1,706 |
| `BenchmarkHashWrite` | 5,000 回の Hash 書き込み | 1,312,209 | 1,464,584 | 1,418 |
| `BenchmarkStringAppend` | 2,000 回の文字列追加 | 397,393 | 319,203 | 3,388 |
| `BenchmarkRescue` | 2,000 回の raise/rescue | 795,952 | 826,358 | 9,384 |

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
| `BenchmarkArithmeticLoop` | 31,383 | 1,376 | 843,317 | 121,697 |
| `BenchmarkMethodCall` | 41,403 | 1,397 | 1,645,429 | 123,432 |
| `BenchmarkArrayMap` | 62,114 | 1,706 | 2,747,425 | 421,086 |
| `BenchmarkHashWrite` | 16,450 | 1,418 | 2,602,342 | 1,464,584 |
| `BenchmarkStringAppend` | 13,383 | 3,388 | 2,523,895 | 319,203 |
| `BenchmarkRescue` | 23,390 | 9,384 | 972,453 | 826,358 |

メソッド呼び出しの割り当ては約 30 分の 1、メモリ使用量は約 13 分の 1。
残る割り当ての大半は `State` の初期化（1 回あたり約 1,380）で、
実行ループ自体の割り当てはほぼなくなっている。

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

### 改善実績: 命令ディスパッチ

インタプリタのホットループから以下を除去した。

- 命令ごとの `map[int]int` による行番号引き（`int32` 配列に事前展開）
- 制限が未設定のときの命令数・時間チェック（context も 1024 命令ごと）
- private メソッドを持たないプログラムでの private 検査
- 引数バッファのゼロクリア（呼び出し側が全要素を上書きする）
- フレームごとの 4 つの `defer`（1 つに統合）
- 1 引数 SEND の文字列 switch（irep デコード時に演算子 ID を事前計算）
- 整数同士の四則演算・比較（`int64` 専用の高速パス）

`BenchmarkArithmeticLoop` は 2.47ms → 0.86ms、`BenchmarkMethodCall` は
1.89ms → 0.95ms に短縮した。

### 改善実績: オブジェクトとペイロードの同一アロケーション

String / Symbol / Array / Hash / Range / Block / Exception は
`Object` とペイロードを別々に割り当てていた。ペイロードを `Object` と
同じホルダー構造体に埋め込み、`Object.data` から同じアロケーション内を
指すようにして 2 回の割り当てを 1 回にまとめた。

| ベンチマーク | allocs/op 変更前 | allocs/op 変更後 |
| --- | ---: | ---: |
| `BenchmarkStringAppend` | 5,392 | 3,388 |
| `BenchmarkRescue` | 13,387 | 9,384 |
| `BenchmarkArrayMap` | 1,913 | 1,708 |

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
go-mrbvm  10 iterations  479.9915ms  20.83 ops/s
mruby     10 iterations  292.215ms   34.22 ops/s
ratio     go-mrbvm/mruby = 1.64x
```

C mruby の計測にはプロセス起動時間が含まれるため、比率は目安。
今後の最適化対象:

- `Value` のボクシングとインターフェース割り当て
- メソッド探索のインラインキャッシュ
- GC のヒープ返却（`madvise`）を減らす割り当て削減
- `Value` のボクシング（`Kind` と `bits` の統合、24 → 16 バイト）
- `State` 初期化コスト（ベンチマークでは最大の割り当て要因）

## 再現方法

```sh
go test ./mrbvm -run '^$' -bench . -benchtime=300ms -count=1
go run ./cmd/vmbench -n 5 -compare benchmark/mixed.rb
```
