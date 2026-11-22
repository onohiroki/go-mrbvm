class C
  def foo
    1
  end
  alias_method :bar, :foo
end
p C.new.bar

class D
  def foo
    2
  end
end
D.class_eval do
  alias_method :bar, :foo
  def baz
    3
  end
end
p D.new.baz
p D.new.bar

class E
  def foo
    4
  end
end
E.class_eval { remove_method :foo }
p E.new.respond_to?(:foo)

obj = Object.new
def obj.x
  @x
end
def obj.x=(value)
  @x = value
end
obj.x = 6
p obj.x

e = Object.new
def e.ary=(value)
  @ary = value
end
def e.ary
  @ary
end
ary = [1, 2]
e.ary = ary
p e.ary

arr = [7, 8]
Array.class_eval do
  alias_method :__old_aref, :[]
  def [](*args)
    :overridden
  end
end
begin
  p arr.__send__(:[], 0)
ensure
  Array.class_eval do
    alias_method :[], :__old_aref
    remove_method :__old_aref
  end
end
p arr.__send__(:[], 0)

a = [1, 2, 3, 4, 5]
a[1, 2] = [9, 8, 7]
p a
a[3..-1] = 6
p a
a[1, 0] = a
p a.length
p a
