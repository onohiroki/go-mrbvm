class Bag
  include Enumerable
  def initialize(*values)
    @values = values
  end
  def each(&block)
    @values.each(&block)
    self
  end
end

b = Bag.new(3, 1, 2)
p b.to_a
p b.map { |x| x * 2 }
p b.collect { |x| x + 1 }
p b.select { |x| x > 1 }
p b.find_all { |x| x > 1 }
p b.reject { |x| x > 1 }
p b.find { |x| x == 1 }
p b.detect { |x| x == 2 }
p b.include?(2)
p b.member?(9)
p b.min
p b.max
p b.minmax
p b.sort
p b.sort { |x, y| y <=> x }
p b.inject(0) { |sum, x| sum + x }
p b.inject { |sum, x| sum + x }
p b.inject(:+)
p b.count
p b.count(1)
p b.count { |x| x > 1 }
p b.all? { |x| x > 0 }
p b.any? { |x| x == 2 }
p b.none? { |x| x > 9 }
p b.one? { |x| x == 1 }
p b.each_with_index.map { |x, i| [x, i] } rescue nil
p b.sort_by { |x| -x }
p b.group_by { |x| x.odd? }
p b.partition { |x| x.odd? }
p b.each_with_object([]) { |x, acc| acc << x * 10 }
p b.first
p b.first(2)
p b.take(2)
p b.drop(1)
p b.each_slice(2).to_a rescue nil

h = {a: 1, b: 2}
p h.map { |k, v| [k, v * 2] }
p h.select { |k, v| v > 1 }
p h.sort_by { |k, v| -v }
p h.min_by { |k, v| v }
p h.max_by { |k, v| v }
