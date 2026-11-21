module Typle
  # src/lib/devices.ts.
  module Devices
    DEFAULT_DEVICE_ID = "default"

    module_function

    def default_device(name) = { "id" => DEFAULT_DEVICE_ID, "name" => name, "layout" => "" }

    def device?(value)
      return false unless Value.object_like?(value)

      id = Value.prop(value, "id")
      name = Value.prop(value, "name")
      layout = Value.prop(value, "layout")
      id.is_a?(String) && !id.empty? && Js.length(id) <= 100 &&
        name.is_a?(String) && !Js.trim(name).empty? && Js.length(name) <= Limits::MAX_DEVICE_NAME &&
        layout.is_a?(String) && Js.length(layout) <= Limits::MAX_DEVICE_LAYOUT
    end

    def decode_devices(value)
      if !value.is_a?(Array) || value.size > Limits::MAX_DEVICES || !value.all? { |item| device?(item) } ||
         value.any? { |item| item["id"] == DEFAULT_DEVICE_ID } || value.map { |item| item["id"] }.uniq.size != value.size
        raise AppError, "deviceDataInvalid"
      end

      value.map { |item| { "id" => item["id"], "name" => Js.trim(item["name"]), "layout" => Js.trim(item["layout"]) } }
    end
  end
end
