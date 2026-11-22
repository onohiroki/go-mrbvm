p "32143".to_i
p "a".to_i(16)
p "100".to_i(2)
p "1_000".to_i
p "1_2__3".to_i
p "68_".to_i
p " _68".to_i
p "abc".index("a")
p "abcabc".index("a", 1)
p "hello".index("", 5)
p "hello".index("l", -2)
p "abcabc".rindex("a")
p "abcabc".rindex("a", 1)
p "abc".rindex("", 2)
s = "abc"
s[1] = "X"
p s
s = "abc"
s[1, 2] = "YZ"
p s
s = "abc"
s[1, 2] = ""
p s
p "abc"[4..5]
p "a" * 2.1
p "a" * 0
begin
  p "a" * -1
rescue => e
  p e.class
end
p "1_2.3__4".to_f
p "68._7".to_f
p " 68 ".to_f
s = "a\nb\nc"
lines = []
s.each_line { |l| lines << l }
p lines
s = "abc"
p s.chop!
p s
s = "abc\n"
p s.chomp!
p s
p "abc".reverse!
p "aBc".capitalize!
p "abc".to_sym
