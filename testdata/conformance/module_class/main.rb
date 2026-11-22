module M
  def hi
    :hi
  end
end

class C
  include M
end
p C.new.hi
p C.include?(M)

p Module.new.class
p M.class

class D
  def self.inherited(sub)
    @sub = sub
  end
  def self.sub
    @sub
  end
end
class E < D
end
p D.sub == E

module F
  def f
    :f
  end
  module_function :f
end
p F.f

class G
  define_method(:x) { 42 }
  attr_reader :y
  def initialize
    @y = 7
  end
end
g = G.new
p g.x
p g.y

class Vis
  def pub
    :pub
  end
  private
  def priv
    :priv
  end
end
v = Vis.new
p v.pub
begin
  v.priv
rescue => e
  p e.class
end
p v.__send__(:priv)
