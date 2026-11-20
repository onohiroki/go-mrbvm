def post(a, *b, c)
  puts a
  puts b.length
  puts c
end
post(1, 2, 3, 4)

def opt(a, b = 1, *c, d)
  puts a
  puts b
  puts c.length
  puts d
end
opt(1, 9)

def anon_rest(*)
end
anon_rest(1, 2)

def anon_kw(**)
end
anon_kw(a: 1)

def anon_block(&)
  [1].each(&)
end
anon_block { |x| puts x }

p [1, 2].map { _1 * 2 }

def forward(...)
  target(...)
end

def target(a, b, k: 1, **rest, &blk)
  puts a
  puts b
  puts k
  puts rest.length
  blk.call if blk
end

forward(1, 2, k: 3, z: 4) { puts "fwd" }

class PBase
  def f(a, k: 1)
    puts a
    puts k
  end
end
class PChild < PBase
  def f(...)
    super(...)
  end
end
PChild.new.f(8, k: 9)
