a, b = 1, 2
puts a + b
x, y = [3, 4]
puts x + y
p, q = 5
puts p
p q
$g, @iv, C = [7, 8, 9]
puts $g
puts @iv
puts C

a, b = 0, 0
[1, 2].each do |v|
  a, b = v, v + 10
end
puts a
puts b

p, *q = [1, 2, 3]
puts p
puts q.length
r, *s, t = [4, 5, 6, 7]
puts r
puts s.length
puts t
