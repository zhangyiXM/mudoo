package mudoo

import (
    "errors"
    "math"
)

const (
    BUFFER_MAX_CAPACITY = 65535 // 缓存包最大长度
    BUFFER_POOL_SIZE    = 10000 // 缓存包对象池大小
    LENGTH_FRAME_SIZE   = 2     // 长度标识头占用字节数
    PID_FRAME_SIZE      = 2     // proto包类型标识占用字节数
)

var (
    _pool = make(chan *Buffer, BUFFER_POOL_SIZE)
)

type Buffer struct {
    pos  int
    data []byte
}

func init() {
    go func() {
        for {
            _pool <- &Buffer{data: make([]byte, 0, BUFFER_MAX_CAPACITY)}
        }
    }()
}

// --------------------------------------------------------

func (buf *Buffer) Data() []byte {
    return buf.data
}

func (buf *Buffer) Length() int {
    return len(buf.data)
}

// --------------------------------------------------------

func (buf *Buffer) ReadBool() (bool, error) {
    b, err := buf.ReadByte()

    if b != byte(1) {
        return false, err
    }

    return true, err
}

func (buf *Buffer) ReadByte() (ret byte, err error) {
    if buf.pos >= len(buf.data) {
        err = errors.New("read byte failed")
        return
    }

    ret = buf.data[buf.pos]
    buf.pos++
    return
}

func (buf *Buffer) ReadBytes() (ret []byte, err error) {
    if (buf.pos + LENGTH_FRAME_SIZE) > len(buf.data) {
        err = errors.New("read bytes header failed")
        return
    }

    size, _ := buf.ReadU16()
    if (buf.pos + int(size)) > len(buf.data) {
        err = errors.New("read bytes data failed")
        return
    }

    ret = buf.data[buf.pos : buf.pos+int(size)]
    buf.pos += int(size)
    return
}

func (buf *Buffer) ReadMessageBytes() (pid uint16, ret []byte, err error) {
    if (buf.pos + LENGTH_FRAME_SIZE) > len(buf.data) {
        err = errors.New("read bytes header failed")
        return
    }

    size, _ := buf.ReadU16()
    if (buf.pos + int(size)) > len(buf.data) {
        err = errors.New("read bytes data failed")
        return
    }

    pid, _ = buf.ReadU16()

    ret = buf.data[buf.pos : buf.pos+int(size)-PID_FRAME_SIZE]
    buf.pos += int(size)
    return
}

func (buf *Buffer) ReadRawBytes() (ret []byte, err error) {
    if buf.pos >= len(buf.data) {
        err = errors.New("read raw bytes failed")
        return
    }

    ret = buf.data[buf.pos:len(buf.data)]
    buf.pos = len(buf.data)
    return
}

func (buf *Buffer) ReadString() (ret string, err error) {
    if (buf.pos + LENGTH_FRAME_SIZE) > len(buf.data) {
        err = errors.New("read string header failed")
        return
    }

    size, _ := buf.ReadU16()
    if (buf.pos + int(size)) > len(buf.data) {
        err = errors.New("read string data failed")
        return
    }

    bytes := buf.data[buf.pos : buf.pos+int(size)]
    buf.pos += int(size)
    ret = string(bytes)
    return
}

func (buf *Buffer) ReadU16() (ret uint16, err error) {
    if (buf.pos + 2) > len(buf.data) {
        err = errors.New("read uint16 failed")
        return
    }

    raw := buf.data[buf.pos : buf.pos+2]
    ret = uint16(raw[0])<<8 | uint16(raw[1])
    buf.pos += 2
    return
}

func (buf *Buffer) ReadS16() (int16, error) {
    ret, err := buf.ReadU16()
    return int16(ret), err
}

func (buf *Buffer) ReadU24() (ret uint32, err error) {
    if (buf.pos + 3) > len(buf.data) {
        err = errors.New("read uint24 failed")
        return
    }

    raw := buf.data[buf.pos : buf.pos+3]
    ret = uint32(raw[0])<<16 | uint32(raw[1])<<8 | uint32(raw[2])
    buf.pos += 3
    return
}

func (buf *Buffer) ReadS24() (int32, error) {
    ret, err := buf.ReadU24()
    return int32(ret), err
}

func (buf *Buffer) ReadU32() (ret uint32, err error) {
    if (buf.pos + 4) > len(buf.data) {
        err = errors.New("read uint32 failed")
        return
    }

    raw := buf.data[buf.pos : buf.pos+4]
    ret = (uint32(raw[0]) << 24) | (uint32(raw[1]) << 16) | (uint32(raw[2]) << 8) | uint32(raw[3])
    buf.pos += 4
    return
}

func (buf *Buffer) ReadS32() (int32, error) {
    ret, err := buf.ReadU32()
    return int32(ret), err
}

func (buf *Buffer) ReadU64() (ret uint64, err error) {
    if (buf.pos + 8) > len(buf.data) {
        err = errors.New("read uint64 failed")
        return
    }

    ret = 0
    raw := buf.data[buf.pos : buf.pos+8]
    for i, v := range raw {
        ret |= uint64(v) << uint((7-i)*8)
    }
    buf.pos += 8
    return
}

func (buf *Buffer) ReadS64() (int64, error) {
    ret, err := buf.ReadU64()
    return int64(ret), err
}

func (buf *Buffer) ReadFloat32() (float32, error) {
    bits, err := buf.ReadU32()
    if err != nil {
        return float32(0), err
    }

    ret := math.Float32frombits(bits)
    if math.IsNaN(float64(ret)) || math.IsInf(float64(ret), 0) {
        return 0, nil
    }

    return ret, nil
}

func (buf *Buffer) ReadFloat64() (float64, error) {
    bits, err := buf.ReadU64()
    if err != nil {
        return float64(0), err
    }

    ret := math.Float64frombits(bits)
    if math.IsNaN(ret) || math.IsInf(ret, 0) {
        return 0, nil
    }

    return ret, nil
}

func (buf *Buffer) WriteZeros(n int) {
    for i := 0; i < n; i++ {
        buf.data = append(buf.data, byte(0))
    }
}

func (buf *Buffer) WriteBool(v bool) {
    if v {
        buf.data = append(buf.data, byte(1))
    } else {
        buf.data = append(buf.data, byte(0))
    }
}

func (buf *Buffer) WriteByte(v byte) {
    buf.data = append(buf.data, v)
}

func (buf *Buffer) WriteBytes(v []byte) {
    buf.WriteU16(uint16(len(v)))
    buf.data = append(buf.data, v...)
}

func (buf *Buffer) WriteRawBytes(v []byte) {
    buf.data = append(buf.data, v...)
}

func (buf *Buffer) WriteString(v string) {
    bytes := []byte(v)
    buf.WriteU16(uint16(len(bytes)))
    buf.data = append(buf.data, bytes...)
}

func (buf *Buffer) WriteU16(v uint16) {
    buf.data = append(buf.data, byte(v>>8), byte(v))
}

func (buf *Buffer) WriteS16(v int16) {
    buf.WriteU16(uint16(v))
}

func (buf *Buffer) WriteU24(v uint32) {
    buf.data = append(buf.data, byte(v>>16), byte(v>>8), byte(v))
}

func (buf *Buffer) WriteU32(v uint32) {
    buf.data = append(buf.data, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

func (buf *Buffer) WriteS32(v int32) {
    buf.WriteU32(uint32(v))
}

func (buf *Buffer) WriteU64(v uint64) {
    buf.data = append(buf.data, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

func (buf *Buffer) WriteS64(v int64) {
    buf.WriteU64(uint64(v))
}

func (buf *Buffer) WriteFloat32(f float32) {
    v := math.Float32bits(f)
    buf.WriteU32(v)
}

func (buf *Buffer) WriteFloat64(f float64) {
    v := math.Float64bits(f)
    buf.WriteU64(v)
}

func Reader(data []byte) *Buffer {
    return &Buffer{data: data}
}

func Writer() *Buffer {
    return <-_pool
}
