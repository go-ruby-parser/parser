h = Hash.new(0)
h[:a] += 1
h[:a] += 1
puts h[:a]
puts h[:x]
puts h.size