class A
  def foo
    1
  end
  alias bar foo
  alias :baz :foo
end
a = A.new
p a.bar
p a.baz

class A
  undef foo
end
p A.new.respond_to?(:foo)
