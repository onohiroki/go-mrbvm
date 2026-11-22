s = "a"
s << "b"
p s
t = s.dup
s << "c"
p s
p t

u = ""
10.times { |i| u << i.to_s }
p u
p u.size
p u[3]

v = "hello"
v << " world"
p v.include?("world")
p v.split(" ")
p v.upcase
v.upcase!
p v
p v.dup

w = "x"
w += "y"
p w
p s
