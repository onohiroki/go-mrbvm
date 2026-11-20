def f(a, b = 2, *rest)
  puts a
  puts b
  puts rest.length
end
f(1)
f(1, 3)
f(1, 3, 4, 5)

def sign(x)
  if x > 0
    "positive"
  elsif x < 0
    "negative"
  else
    "zero"
  end
end
puts sign(1)
puts sign(-1)
puts sign(0)
