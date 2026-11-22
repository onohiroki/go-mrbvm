class A
  X = 1
  def initialize
    @a = 2
    @b = 3
  end
  def foo; 1; end
end
class B < A
  def bar; 2; end
end
b = B.new
p b.instance_variables.sort
p b.instance_variable_get(:@a)
p b.instance_variable_get(:@b)
p b.instance_variable_defined?(:@c)
b.instance_variable_set(:@c, 4)
p b.instance_variable_get(:@c)
p B.const_get(:X)
B.const_set(:Y, 5)
p B.const_get(:Y)
p B.ancestors.include?(A)
p B.ancestors.include?(Object)
p b.methods.include?(:foo)
p b.methods.include?(:bar)
p b.respond_to?(:foo)
p b.respond_to?(:nope)
