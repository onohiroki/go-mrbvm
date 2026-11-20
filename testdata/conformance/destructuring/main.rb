(a, b), c = [1, 2]
p a
p b
p c

def f((x, y), z)
  p x
  p y
  p z
end
f([3, 4], 5)

[[1, 2, 3]].each { |a,| p a }

case { a: 1, b: 2 }
in { a: 1, b: val }
  p val
end

begin
  raise "x"
rescue *[ArgumentError, RuntimeError] => e
  p e.message
end

def m
  return 7, 8
end
p m
