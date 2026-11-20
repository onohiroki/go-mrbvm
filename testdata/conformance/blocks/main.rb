def outer
  a = 1
  [1].each do |i|
    [2].each do |j|
      puts a + i + j
    end
  end
end
outer

def twice
  yield 1
  yield 2
end
twice { |v| puts v * 2 }

def with_block(&b)
  b.call(5)
end
with_block { |v| puts v + 1 }

def each_dep
  [1, 2].each { |x| yield x }
end
each_dep { |x| puts x * 10 }

r = [1, 2, 3].each do |x|
  break x * 10 if x == 2
end
puts r

[1, 2, 3].each do |x|
  next if x == 2
  puts x
end
