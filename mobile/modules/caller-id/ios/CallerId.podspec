Pod::Spec.new do |s|
  s.name           = 'CallerId'
  s.version        = '1.0.0'
  s.summary        = 'Caller ID: hands the customer phone book to the Call Directory extension'
  s.description    = 'Local Expo module: writes the phone book to the App Group and reloads the CallKit Call Directory extension (TEC-1208).'
  s.license        = 'UNLICENSED'
  s.author         = 'Olexfilms'
  s.homepage       = 'https://olexfilms.com'
  s.platforms      = { :ios => '16.4' }
  s.swift_version  = '5.9'
  s.source         = { git: '' }
  s.static_framework = true

  s.dependency 'ExpoModulesCore'
  s.frameworks = 'CallKit'

  s.pod_target_xcconfig = {
    'DEFINES_MODULE' => 'YES',
    'SWIFT_COMPILATION_MODE' => 'wholemodule'
  }

  s.source_files = "**/*.{h,m,swift}"
end
