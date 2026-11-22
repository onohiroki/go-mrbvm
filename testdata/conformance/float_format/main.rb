p 0.0
p(-0.0)
p 1.0
p 100.0
p 1.5
p 1e20
p 1e-7
p 1.0 / 0.0
p(-1.0 / 0.0)
p 0.0 / 0.0
p 1.2345678901234567
puts 1.0
puts 1e20
puts 1.0 / 0.0

h = {}
h[0.0] = "zero"
p h[0.0]
p h[-0.0]
h[1] = "int"
p h[1.0]
p h.keys
p h.size
