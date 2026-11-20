def f
  a = 0
  for x in [1, 2, 3]
    a = a + x
    next if x == 2
    a = a + 10
  end
  a
end
p f

for i in [1, 2]
  for j in [3, 4]
    puts i * 10 + j
  end
end

r = for k in [1, 2, 3]
  break k * 100 if k == 2
end
p r

[5].each do |v|
  for x in [1, 2]
    puts v + x
  end
end

class E
  def each
    yield 1
    yield 2
  end
end
for x in E.new
  puts x
end

for $g in [1, 2]
end
p $g

for @iv in [7]
end
p @iv
