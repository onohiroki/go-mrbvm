a = []
for i in 0...2
  [10, 20].each do |j|
    k = j + i
    a << k
  end
end
p a

b = []
[1].each do
  for x in [2, 3]
    [4].each { |y| b << x + y }
  end
end
p b
