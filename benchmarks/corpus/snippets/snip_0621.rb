w = FFT.hann(4)
sg = FFT.spectrogram([1, 2, 3, 4, 5, 6, 7, 8].map { |x| x.to_f }, 4, 2, w, 1.0)
p [sg.length, sg[0].length]