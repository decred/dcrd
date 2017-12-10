Pod::Spec.new do |s|
  s.name         = "Dcrd"
  s.version      = "0.0.1" # Version should be rendered based on dcrd version
  s.summary      = "Dcrd Library - iOS"
  s.homepage     = "https://decred.org/"
  # s.license      = "MIT (example)" 
  # s.license      = { :type => "MIT", :file => "FILE_LICENSE" }
  s.author             = { "Ricardo Geraldes" => "rgeraldes24@gmail.com" }
  s.platform     = :ios, "9.0" # deployment target
  s.source       = { :git => "https://github.com/decred/dcrd.git", :tag => "#{s.version}" }
end