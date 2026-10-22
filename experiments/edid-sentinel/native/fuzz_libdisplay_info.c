/* EDID Sentinel: libFuzzer harness for local, authorized, isolated parser testing.
 * Compile the target library with sanitizers as well, not just this harness.
 * No HDMI, DDC or kernel device access is performed.
 */
#include <stddef.h>
#include <stdint.h>
#include <libdisplay-info/info.h>

int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
    struct di_info *info = di_info_parse_edid(data, size);
    if (info != NULL) {
        (void)di_info_get_edid(info);
        di_info_destroy(info);
    }
    return 0;
}
