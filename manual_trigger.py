## NOTE: THIS FILE IS FOR MANUALLY TESTING THE CONFIG UPDATE TRIGGER ONLY ; NOT TO BE USED IN PROD

import ctypes
import sys

kernel32 = ctypes.WinDLL("kernel32", use_last_error=True)

EVENT_MODIFY_STATE = 0x0002
EVENT_NAME = "FlowkeyConfigReload"

kernel32.OpenEventW.restype = ctypes.c_void_p
kernel32.OpenEventW.argtypes = [ctypes.c_uint32, ctypes.c_bool, ctypes.c_wchar_p]

handle = kernel32.OpenEventW(EVENT_MODIFY_STATE, False, EVENT_NAME)

if not handle:
    err = ctypes.get_last_error()
    print(f"Failed to open event '{EVENT_NAME}' (error {err}) — is the daemon running?")
    sys.exit(1)

if kernel32.SetEvent(handle):
    print("Reload signal sent.")
else:
    print("SetEvent failed.")
    sys.exit(1)

kernel32.CloseHandle(handle)