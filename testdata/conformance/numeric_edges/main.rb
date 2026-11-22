p 10.to_s(2)
p 255.to_s(16)
p((-255).to_s(16))
p 10.to_s(36)
p((2 ** 70).to_s(16))
begin; p 10.to_s(1); rescue => e; p e.class; p e.message; end

p 7.0.divmod(2)
p((-7.0).divmod(2))
p 7.0.divmod(-2)
p 7.5.divmod(2)
p 7.0.fdiv(2)
p 1.0.fdiv(0)
begin; p 7.0.divmod(0); rescue => e; p e.class; end

p 12.lcm(18)
p((-12).lcm(18))
p 0.lcm(5)
begin; p((2 ** 62).lcm(3)); rescue => e; p e.class; end

p 123.456.round
p 123.456.round(1)
p 123.456.round(-1)
p 123.456.floor(2)
p 123.456.floor(-1)
p 123.456.ceil(2)
p 123.456.ceil(-1)
p((-123.456).round(1))
p 1.25.round(1)
p 1.35.round(1)
begin; p((1.0 / 0.0).floor); rescue => e; p e.class; end
begin; p((0.0 / 0.0).round); rescue => e; p e.class; end
p((1.0 / 0.0).round(2))

p 7 & 3
p 7 | 8
p 7 ^ 3
p ~7
p 1 << 70
p(-1 >> 70)

p 10.clamp(1, 5)
begin; p 1.clamp(5, 3); rescue => e; p e.class; p e.message; end
