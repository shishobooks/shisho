import Darwin

/// The app's own memory, as iOS counts it against the limit past which it
/// ends the app.
enum MemoryFootprint {
    static var bytes: Int {
        var info = task_vm_info_data_t()
        var count = mach_msg_type_number_t(MemoryLayout<task_vm_info_data_t>.size / MemoryLayout<natural_t>.size)
        let result = withUnsafeMutablePointer(to: &info) {
            $0.withMemoryRebound(to: integer_t.self, capacity: Int(count)) {
                task_info(mach_task_self_, task_flavor_t(TASK_VM_INFO), $0, &count)
            }
        }
        return result == KERN_SUCCESS ? Int(info.phys_footprint) : -1
    }

    static var megabytes: Int {
        let bytes = bytes
        return bytes < 0 ? -1 : bytes / 1_048_576
    }
}
