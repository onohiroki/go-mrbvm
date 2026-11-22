class K
  attr_reader :v
  def initialize(v)
    @v = v
  end
  def hash
    @v % 3
  end
  def eql?(other)
    other.is_a?(K) && @v == other.v
  end
  def ==(other)
    other.is_a?(K) && @v == other.v
  end
  def to_s
    "K#{@v}"
  end
  alias inspect to_s
end

h = {}
h[K.new(2)] = :a
h[K.new(5)] = :b
h[K.new(2)] = :c
p h.size
p h[K.new(2)]
p h[K.new(5)]
p h.keys.map(&:v)
p h.key?(K.new(5))

k = "_key"
s = {}
s[k] = 1
p s.keys[-1].frozen?
p k.frozen?
p s.keys[-1].equal?(k)
p s["_key"]

d = Hash.new { |hh, kk| hh[kk] = kk * 2 }
p d[3]
p d[3]
p d.default(4)
p d.keys

m = { a: 1 }
p m.merge({ b: 2 }, { a: 3 })
p m.shift
p m
