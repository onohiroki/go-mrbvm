p Proc.new { 1 }.call
p proc { 2 }.call
p lambda { 3 }.call
p Proc.new { |x, *y, z| [x, y, z] }.call
p Proc.new { |x, *y, z| [x, y, z] }.call(1)
p Proc.new { |x, *y, z| [x, y, z] }.call(1, 2, 3, 4)
p Proc.new { |x, y| [x, y] }.call(1, 2, 3)
begin
  Proc.new
rescue => e
  p e.class
end
begin
  Class.new(1)
rescue => e
  p e.class
end
begin
  class ProcSuperclassTest < 1
  end
rescue => e
  p e.class
end
