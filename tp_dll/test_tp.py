"""
一个最小的 mock 设备，用回调收集 TP 层要发的 CAN 帧，并可主动注入接收帧。
运行前先用 `go build -buildmode=c-shared -o tp_layer.dll ./tp_dll` 生成 DLL。
"""

import ctypes
import os
import queue
import threading
import time

# 1. 加载 DLL
dll_path = os.path.abspath(os.path.join(os.path.dirname(__file__), "../tp_layer.dll"))
lib = ctypes.CDLL(dll_path)

# 2. 定义 C 类型
GoUint32 = ctypes.c_uint32
GoUint8 = ctypes.c_uint8
GoInt = ctypes.c_int

# 3. Callback 类型
TX_CALLBACK_TYPE = ctypes.CFUNCTYPE(None, GoUint32, ctypes.POINTER(GoUint8), GoInt)


class MockCanDevice:
    """简单的 mock，收集 Go 侧发出的 CAN 帧，并支持向 Go 注入帧。"""

    def __init__(self):
        self.tx_queue = queue.Queue()  # Go -> 设备
        # 保持回调引用，避免被 GC
        self.cb_func = TX_CALLBACK_TYPE(self._on_tx_from_go)

    def _on_tx_from_go(self, can_id, data_ptr, length):
        frame = bytes(data_ptr[:length])
        print(f"[MockDevice] Go 请求发送: ID=0x{can_id:03X} len={length} data={frame.hex().upper()}")
        self.tx_queue.put((can_id, frame))

    def inject_single_frame(self, can_id, payload: bytes):
        """构造一个单帧 SF 并注入到 Go 的输入（最大 7 字节 payload）。"""
        if len(payload) > 7:
            raise ValueError("SF payload must be <= 7 bytes")
        dl = len(payload)
        sf = bytes([dl]) + payload + bytes(7 - dl)
        c_buf = (GoUint8 * len(sf))(*sf)
        lib.GoInputCanFrame(can_id, c_buf, len(sf))
        print(f"[MockDevice] 注入接收帧: ID=0x{can_id:03X} data={sf.hex().upper()}")


# 4. 设置函数签名
lib.GoInitTp.argtypes = [GoUint32, GoUint32, GoUint8, TX_CALLBACK_TYPE]
lib.GoInputCanFrame.argtypes = [GoUint32, ctypes.POINTER(GoUint8), GoInt]
lib.GoSendTp.argtypes = [ctypes.POINTER(GoUint8), GoInt]
lib.GoRecvTp.argtypes = [ctypes.POINTER(GoUint8), GoInt, GoInt]
lib.GoRecvTp.restype = GoInt


def main():
    print("--- 开始测试 TP 导出 ---")
    dev = MockCanDevice()

    # RxID=0x7E8, TxID=0x7E0, isFD=0
    print("1) 初始化 TP (GoInitTp)")
    lib.GoInitTp(0x7E8, 0x7E0, 0, dev.cb_func)

    # 发送长数据，触发 Go -> callback
    print("\n2) 测试 GoSendTp，观察回调收到的帧")
    payload = bytes(range(20))  # 20 字节，需分帧
    c_payload = (GoUint8 * len(payload))(*payload)
    lib.GoSendTp(c_payload, len(payload))

    # 等待 Go 的发送事件
    time.sleep(0.5)
    print(f"[MockDevice] 收到 {dev.tx_queue.qsize()} 个待发送帧（按需检查内容）")

    # 注入一条 SF，验证 GoRecvTp 拼包
    print("\n3) 注入单帧，验证 GoRecvTp")
    dev.inject_single_frame(0x7E8, b"\xAA\xBB\xCC")
    recv_buf = (GoUint8 * 64)()
    n = lib.GoRecvTp(recv_buf, 64, 1000)  # 1s 超时
    if n > 0:
        data = bytes(recv_buf[:n])
        print(f"[MockDevice] GoRecvTp 成功: {data.hex().upper()}")
    else:
        print(f"[MockDevice] GoRecvTp 无数据/超时 (n={n})")

    lib.GoCloseTp()
    print("\n--- 测试结束 ---")


if __name__ == "__main__":
    main()
