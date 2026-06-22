i = Image.new(3, 2); i.set(1, 1, 99, 88, 77)
j = Image.decode(i.to_png)
p [j.width, j.height, j.get(1, 1)]