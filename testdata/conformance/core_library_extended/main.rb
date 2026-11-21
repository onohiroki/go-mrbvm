a = [1, 2, 3, 4, 5]
p a.take(2)
p a.drop(2)
p a.take_while { |x| x < 3 }
p a.drop_while { |x| x < 3 }
r = []
a.each_slice(2) { |s| r << s }
p r
r = []
a.each_cons(3) { |s| r << s }
p r
p a.zip([9, 8, 7])
p a.flat_map { |x| [x, x * 10] }
p a.group_by { |x| x % 2 }
p a.partition { |x| x.odd? }
p a.each_with_object([]) { |x, acc| acc << x * 2 }
p a.sort_by { |x| -x }
p a.minmax
p a.sum
p a.sum { |x| x * 2 }

h = {a: 1, b: 2, c: 3}
p h.map { |k, v| [k, v * 2] }
p h.select { |k, v| v.odd? }
p h.reject { |k, v| v.odd? }
p h.find { |k, v| v == 2 }
p h.sort_by { |k, v| -v }
p h.group_by { |k, v| v.odd? }
p h.each_with_object({}) { |(k, v), acc| acc[k] = v * 10 }
p h.key(2)
p h.dig(:a)
p h.transform_values { |v| v * 3 }
p h.transform_keys { |k| k.to_s }

s = "hello world"
p s.chomp
p "abc\n".chomp
p s.chop
p "abc".chop
p s.capitalize
p s.swapcase
p "abc".ljust(5, ".")
p "abc".rjust(5, ".")
p "abc".center(7, "-")
p "ab" * 3
p "%d-%s" % [7, "x"]
p "A".ord
p "az".succ
p s.count("lo")
p s.delete("l")
p s.tr("lo", "01")
p "a\nb\nc".lines

p 7.div(2)
p(-7.div(2))
p 7.divmod(2)
p 7.fdiv(2)
p 5.bit_length
p 123.digits
p 2.pow(10)
p 5.between?(1, 9)
p 12.clamp(1, 10)

p 3.7.round
p 3.2.floor
p 3.2.ceil
p 3.7.to_i
p 1.5.nan?
p (1.0 / 0).infinite?
