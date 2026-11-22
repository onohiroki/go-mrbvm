class MyArray < Array; end
class MyHash < Hash; end
class MyString < String; end

a = MyArray.new
a << 1
d = a.dup
p d.class
p d
p d.equal?(a)

h = MyHash.new
h[:x] = 1
p h.dup.class
p h.dup

s = MyString.new("x")
p s.dup.class
p s.dup

class WithIvar
  def initialize
    @x = 1
  end
  attr_reader :x
end
w = WithIvar.new
p w.dup.x
p w.dup.equal?(w)
