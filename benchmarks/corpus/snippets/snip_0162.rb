h = Hash.new { |hash, k| hash[k] = k * 10 }
puts h[5]
puts h.size