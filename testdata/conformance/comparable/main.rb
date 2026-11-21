class Version
  include Comparable
  attr_reader :value
  def initialize(value)
    @value = value
  end
  def <=>(other)
    @value <=> other.value
  end
  def to_s
    "v#{@value}"
  end
end

a = Version.new(1)
b = Version.new(2)
c = Version.new(2)
d = Version.new(3)
p a < b
p a <= b
p b == c
p b > a
p b >= c
p d > c
p b.between?(a, d)
p b.between?(c, c)
p b.clamp(a, c).value
p d.clamp(a, c).value
p [a, d, b].sort.map(&:to_s)

p 3 < 4
p "a" < "b"
p 2.between?(1, 3)
p 12.clamp(1, 10)
p "m".between?("a", "z")
