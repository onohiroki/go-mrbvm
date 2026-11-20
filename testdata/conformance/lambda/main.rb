x = 1
l = ->(y) { x + y }
puts l.call(2)

def m
  inner = -> { return 5 }
  inner.call
  9
end
p m

l2 = -> { next 7 }
p l2.call

l3 = ->(a, b = 2) { a + b }
p l3.call(1)
p l3.call(1, 4)

pr = proc { |v| v * 3 }
p pr.call(2)
