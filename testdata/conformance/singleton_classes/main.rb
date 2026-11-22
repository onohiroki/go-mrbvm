arr = [1, 2]
class << arr
  def total
    self[0] + self[1]
  end
end
p arr.total

str = "abc"
class << str
  def shout
    upcase
  end
end
p str.shout

class SubArray < Array
end
sub = SubArray[1, 2, 3]
p sub.class
p sub.is_a?(Array)
p sub.size
p sub[1]
p Array.include?(Enumerable)
p SubArray.include?(Enumerable)

p Array[1, 2, 3]
p([1] + [2])
p([1] * 3)
p([1, 2, 3][1, 2])
p([1, 2, 3][1.9])
p([1, 2, 3][1..])
p([1, 2, 3][..1])
p([0, [0, 1], {foo: 0}] * ", ")


def splat_rescue(*classes)
  begin
    raise ArgumentError, "boom"
  rescue *classes => e
    [e.class, e.message]
  end
end
p splat_rescue(ArgumentError)
p splat_rescue(TypeError, ArgumentError)
