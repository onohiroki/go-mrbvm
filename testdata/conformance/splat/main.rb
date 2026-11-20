def f(*a)
  puts a.length
end

a = [1, 2, 3]
f(*a)
f(1, *a)
f(1, *a, 9)

def g(&b)
  f(*[4, 5], &b)
end
g { puts "unused" }

class Base
  def initialize(a, b)
    @a = a
    @b = b
  end
  def show
    puts @a
    puts @b
  end
end

class Child < Base
  def initialize(a)
    super(a, a + 1)
  end
end
Child.new(8).show

class SplatChild < Base
  def initialize(a)
    super(*[a, a + 20])
  end
end
SplatChild.new(5).show
