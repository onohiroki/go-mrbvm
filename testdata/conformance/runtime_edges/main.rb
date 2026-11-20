# shareable_constant_value: literal
m = class ClassExpr; :body_value end
p m
p(class EmptyClass end)

class OuterForAbsolute
  class ::TopLevelFromClass
  end
end
p TopLevelFromClass

class Setter
  def x=(value)
    99
  end
end
setter = Setter.new
p(setter.x = 5)
p(setter.x = 6)

module ConstOps
  VALUE ||= 3
  VALUE ||= 4
  TOTAL = 5
  TOTAL += 6
end
p ConstOps::VALUE
p ConstOps::TOTAL

class Holder
end
Holder::PATH = "a"
Holder::COUNT = 1
p Holder::PATH
p Holder::COUNT

p 123456789012345678901234567890
p(-123456789012345678901234567890)
p 123456789012345678901234567890 + 1

p [1].map { while true; break :done; end }
idx = 0
p [1, 2, 3].map {
  idx += 1
  next idx if idx < 3
  idx
}
