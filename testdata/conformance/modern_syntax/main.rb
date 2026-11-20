x = nil
p x&.to_s
y = "ab"
p y&.upcase
p (..3).include?(2)
p (1..).include?(5)
a = [1, *[2, 3], 4]
p a
h = { a: 1 }
p({ **h, b: 2 })
p :"a#{1}b"
class A
end
A::X = 5
p A::X
