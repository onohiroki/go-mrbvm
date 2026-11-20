def f(a, b = 1, k:, m: 2, **rest, &blk)
  puts a
  puts b
  puts k
  puts m
  puts rest.length
  yield if blk
end

f(10, k: 5)
f(10, 20, k: 5, m: 6, z: 7) { puts "block" }

h = { k: 9, q: 1 }
f(1, **h)

l = ->(x:, y: 2) { puts x; puts y }
l.call(x: 3)
l.call(x: 3, y: 4)

[1].each do |v, w: 5|
  puts v
  puts w
end

def ks(*a, k: 1, **rest)
  puts a.length
  puts k
  puts rest.length
end
arr = [1, 2]
h = { k: 7, z: 8 }
ks(*arr, k: 5)
ks(*arr, **h)
ks(**h, k: 9)

class KA
  def initialize(a, k:, **rest)
    puts a
    puts k
    puts rest.length
  end
end
class KB < KA
  def initialize(a)
    super(a, k: 6, z: 2)
  end
end
KB.new(8)
class KC < KA
  def initialize(a)
    super(*[a], k: 7)
  end
end
KC.new(9)
