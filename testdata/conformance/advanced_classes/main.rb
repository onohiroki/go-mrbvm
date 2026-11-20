class C
  x = 10
  [1].each do |i|
    puts x + i
  end
end

class A
end
class A::B
  def x
    1
  end
end
puts A::B.new.x

class K
  def self.base
    "base"
  end
end
class L < K
  def self.base
    super + "-child"
  end
end
puts L.base

class CV
  @@v = 1
  def self.bump
    @@v = @@v + 1
  end
  def self.value
    @@v
  end
end
puts CV.value
CV.bump
puts CV.value

class T
  X = 4
end
puts ::T::X

class SC
  class << self
    def foo
      5
    end
  end
end
puts SC.foo
