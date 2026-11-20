def try(label)
  yield
rescue => e
  p [label, e.class]
end

def kw(a, **k)
  p [a, k]
end
try(1) { kw('a' => 1, b: 2) }
try(2) { kw({'a' => 1}, b: 2) }

def show(hash)
  p hash
end
show('a' => 1)

def rest(*a, **k)
  p [a, k]
end
rest('a' => 1, b: 2)

def target(*a, **k, &b)
  p [a, k, !b.nil?]
end
def fwd(...)
  target(...)
end
def fwd_lead(x, ...)
  target(x, ...)
end
fwd(1, 2, c: 3) { }
fwd_lead(9, 1, 2, c: 3) { }

a = []
limit = 3
err = RuntimeError.new("!")
for i in 0...3
  begin
    limit -= 1
    break unless limit > 0
    a.push i * 4 + 1
    a.push i * 4 + 2
    raise err
  rescue
    a.push i * 4 + 3
    redo
  ensure
    a.push i * 4 + 4
  end
end
p a

def no_block(&nil); end
no_block
begin
  no_block { }
rescue => e
  p e.class
end

raised = proc { super }
begin
  raised.call
rescue NoMethodError
  p :super_outside_method
end
