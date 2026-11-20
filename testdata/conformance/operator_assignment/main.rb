x = 1
x += 4
puts x

$g = nil
$g ||= 7
puts $g
$g &&= $g + 1
puts $g

a = [1]
a[0] = 9
puts a[0]

class Counter
  @@total = 0
  def initialize
    @n = 1
  end
  def bump
    @n += 2
    @@total += 3
  end
  def show
    puts @n
    puts @@total
  end
end
c = Counter.new
c.bump
c.show
