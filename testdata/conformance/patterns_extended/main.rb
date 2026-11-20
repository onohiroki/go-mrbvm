i = 0
begin
  i += 1
end while i < 3
p i

i = 0
begin
  i += 1
  next if i == 2
  p i
end until i >= 3

case 1
in 1 | 2
  p :alternation
end

case 3
in 1 | 2
  p :bad
else
  p :no_alternation
end

case [1, 2, 3]
in [a, *b, c]
  p [a, b, c]
end

case [1, 2]
in [a, *b]
  p [a, b]
end

case [1, 2, 3]
in [*, 2, *]
  p :find
end

case [1, 2, 3, 4]
in [*pre, 3, *post]
  p [pre, post]
end

case [1, 2, 3]
in [*, 9, *]
  p :bad_find
else
  p :no_find
end

def show_hash(hash)
  p hash
end
extra = { "a" => 1 }
show_hash("b" => 2, **extra)

1 => Integer
p :required_ok

begin
  [1, 2] => [a, 3]
  p :bad_required
rescue NoMatchingPatternError
  p :no_match
end

class Deconstructible
  def deconstruct
    [10, 20, 30]
  end
end

case Deconstructible.new
in [x, *rest]
  p [x, rest]
end

case 1
when *[1, 2]
  p :when_splat
end

def keyword_yield
  yield b: true
end
p keyword_yield { |*args| args }

def count_arguments(*args)
  args.size
end
p count_arguments(1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15)
