sum = 0
i = 0
while i < 200000
  sum += i
  i += 1
end

def fib(n)
  if n < 2
    n
  else
    fib(n - 1) + fib(n - 2)
  end
end
fib(22)

h = {}
i = 0
while i < 20000
  h[i] = i * 2
  i += 1
end

a = (1..1000).to_a
i = 0
while i < 200
  a.map { |x| x * 2 }
  i += 1
end

s = ""
i = 0
while i < 5000
  s << "x"
  i += 1
end
