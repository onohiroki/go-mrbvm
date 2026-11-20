class Box
  def x
    @x
  end
  def x=(v)
    @x = v
  end
end

a = [0, 0]
b = Box.new
b.x = 0
a[0], b.x = [5, 6]
puts a[0]
puts b.x

a[1], *rest = [7, 8, 9]
puts a[1]
puts rest.length

a[0] += 10
puts a[0]
a[1] ||= 20
puts a[1]
a[1] &&= a[1] + 1
puts a[1]
b.x += 3
puts b.x
b.x ||= 99
puts b.x

for x, y in [[1, 2], [3, 4]]
  puts x + y
end
for p, *q, r in [[1, 2, 3, 4]]
  puts p
  puts q.length
  puts r
end
