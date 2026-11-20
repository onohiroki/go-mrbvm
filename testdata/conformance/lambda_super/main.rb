class Base
  def no_args
    [:base_no_args]
  end

  def required(x)
    [:base_required, x]
  end

  def optional(x = 1)
    [:base_optional, x]
  end

  def full(x, *rest, key: 1, **opts, &block)
    [:base_full, x, rest, key, opts, block ? block.call : nil]
  end

  def explicit(x)
    [:base_explicit, x]
  end

  def empty
    [:base_empty]
  end

  def nested(x)
    [:base_nested, x]
  end

  def in_block(x)
    [:base_in_block, x]
  end
end

class Child < Base
  def no_args
    -> { super }.call
  end

  def required(x)
    -> { super }.call
  end

  def optional(x = 1)
    -> { super }.call
  end

  def full(x, *rest, key: 1, **opts, &block)
    -> { super }.call
  end

  def explicit(x)
    -> { super(x + 1) }.call
  end

  def empty
    -> { super() }.call
  end

  def nested(x)
    -> { -> { super }.call }.call
  end

  def in_block(x)
    [1].each { -> { super }.call }
  end

  def yielding(x)
    -> { yield x }.call
  end
end

class Escaped < Base
  def required(x)
    -> { super }
  end
end

p Child.new.no_args
p Child.new.required(2)
p Child.new.optional
p Child.new.optional(3)
p Child.new.full(4, 5, 6, key: 7, extra: 8) { :block_value }
p Child.new.explicit(9)
p Child.new.empty
p Child.new.nested(10)
p Child.new.in_block(11)
p Child.new.yielding(12) { |value| [:yielded, value] }

escaped = Escaped.new.required(13)
p escaped.call

def lambda_return
  callable = -> { return 1; 2 }
  callable.call
  3
end
p lambda_return

def lambda_break
  callable = -> { break 1; 2 }
  [callable.call, 3]
end
p lambda_break

def lambda_next
  callable = -> { next 1; 2 }
  [callable.call, 3]
end
p lambda_next

def lambda_keyword_yield
  -> { yield 1, key: 2 }.call
end
p lambda_keyword_yield { |*args| args }

def forward_target(*args, **opts)
  [args, opts]
end

def lambda_forward(...)
  -> { forward_target(...) }.call
end
p lambda_forward(1, 2, key: 3)

begin
  proc { super }.call
rescue NoMethodError
  p :super_outside_method
end
