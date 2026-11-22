h = { a: 1, b: 2 }
puts h[:a]
h.delete(:a)
h[:b] = 3
p h
puts h.size

n = { 1 => "a", 2 => "b" }
puts n[1]
n.delete(1)
n[2] = "c"
p n
puts n.size

f = { 1.5 => "x" }
f[1.5] = "y"
p f

s = { "k" => 1 }
s["k"] = 2
p s

big = {}
i = 0
while i < 100
  big[i] = i
  i += 1
end
puts big.size
puts big[99]
big.delete(0)
big[0] = -1
puts big[0]
puts big.size
