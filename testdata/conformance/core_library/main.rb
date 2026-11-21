# Array
a = [3, 1, 2]
p a.sort
p a.sort { |x, y| y <=> x }
p a.map { |x| x * 2 }
p a.select { |x| x > 1 }
p a.reject { |x| x > 1 }
p a.find { |x| x == 1 }
p a.include?(2)
p a.member?(9)
p a.min
p a.max
p a.min { |x, y| y <=> x }
p a.max { |x, y| y <=> x }
p a.inject(0) { |sum, x| sum + x }
p a.inject { |sum, x| sum + x }
p a.inject(:+)
p a.first
p a.first(2)
p a.last
p a.last(2)
p a.reverse
p a.join("-")
p a.index(2)
p a.rindex(1)
p a.count
p a.count(1)
p a.count { |x| x > 1 }
p a.all? { |x| x > 0 }
p a.any? { |x| x == 2 }
p a.none? { |x| x > 5 }
p a.one? { |x| x == 1 }
b = a.dup
b << 4
b.push 5
b.unshift 0
p b
p b.shift
p b.pop
p b.delete_at(1)
p b.delete(4)
p b
p [1, [2, [3]]].flatten
p [1, 2, 2, 3, 1].uniq
p [1, nil, 2, nil].compact
p [1, 2].concat([3, 4])
p [1, 2, 3, 4].slice(1, 2)
p [1, 2, 3, 4].slice(1..2)
r = [1, 2]
r.reverse!
p r
r.each_index { |i| print i }
puts
[10, 20].each_with_index { |v, i| puts "#{i}:#{v}" }

# Hash
h = {a: 1, b: 2}
p h.keys
p h.values
p h[:a]
p h.fetch(:b)
p h.fetch(:z, 9)
p h.key?(:a)
p h.has_key?(:z)
p h.has_value?(2)
p h.value?(9)
p h.to_a
p h.length
p h.empty?
h.each { |k, v| puts "#{k}=#{v}" }
h.each_key { |k| print k }
puts
h.each_value { |v| print v }
puts
p h.merge({b: 20, c: 3})
p h.merge({a: 10}) { |key, old, new| old + new }
p h.invert
p h.values_at(:b, :z)
g = h.dup
p g.delete(:a)
p g
g[:d] = 4
p g
p({}.empty?)

# String
s = "Hello, World"
p s.empty?
p "".empty?
p s.length
p s.upcase
p s.downcase
p s.include?("World")
p s.start_with?("Hell")
p s.start_with?("x", "He")
p s.end_with?("World")
p s.split(", ")
p s.index("World")
p s.rindex("o")
p s[0, 5]
p s[7..]
p s.slice(0, 5)
p s.sub("World", "Ruby")
p s.gsub("l", "L")
p s.sub("World") { |m| m.upcase }
p "  x ".strip
p "\t x \n".strip
p "a,b,c".split(",")
p "abc".chars
p "ABC".bytes
p "abc".reverse
p "42".to_i
p "3.5".to_f
p "abc".to_sym
t = "ab"
t << "cd"
t.concat("e")
p t

# Integer
5.times { |i| print i }
puts
p 3.upto(5).to_a
p 5.downto(3).to_a
p 1.step(9, 2).to_a
p 7.abs
p(-7.abs)
p 3.even?
p 3.odd?
p 0.zero?
p 3.positive?
p(-3.negative?)
p 10.gcd(4)
p 2 ** 10
p 7 / 2
p 7 % 3
p(-7 % 3)
p 2.succ
p 2.next
p 65.chr

# Object / Comparable
p 1.is_a?(Integer)
p 1.kind_of?(Numeric)
p 1.instance_of?(Integer)
p "x".respond_to?(:upcase)
p "x".respond_to?(:nope)
p 3 <=> 2
p "a" <=> "b"
p 1.equal?(1)
p 1.eql?(1)
p [1, 2] == [1, 2]
p({a: 1} == {a: 1})
p((1..3).include?(2))
p((1...3).include?(3))
p((1..3).size)
p((1...3).size)
