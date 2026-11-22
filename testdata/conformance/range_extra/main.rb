p ("a".."e").to_a
a = []
("a"..."d").each { |c| a << c }
p a
b = []
(1..).each do |i|
  b << i
  break if i >= 5
end
p b
p (1..5).to_a
p (1...5).to_a
p (1..).first
p (1..5).first
p (1..5).last
p (1.0..2.0).begin
p (1..10) == Range.new(1.0, 10.0)
p (1..10).eql?(Range.new(1.0, 10.0))
p ("a".."b").to_s
p ("a".."b").inspect
p (0..).to_s
p (..1).to_s
p (0...).inspect
p (1...3).include?(3)
p (1..3).include?(2)
