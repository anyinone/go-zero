package signalr

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/anyinone/jsoniter"
	"github.com/go-kit/log"
	"github.com/vmihailenco/msgpack/v5"
)

// txtHubProtocol is the JSON based SignalR protocol
type txtHubProtocol struct {
	dbg log.Logger
}

// Protocol specific messages for correct unmarshaling of arguments or results.
// txtInvocationMessage is only used in ParseMessages, not in WriteMessage
type txtInvocationMessage struct {
	Type         int                   `json:"type"`
	Target       string                `json:"target"`
	InvocationID string                `json:"invocationId"`
	Arguments    []jsoniter.RawMessage `json:"arguments"`
	StreamIds    []string              `json:"streamIds,omitempty"`
}

type txtStreamItemMessage struct {
	Type         int                 `json:"type"`
	InvocationID string              `json:"invocationId"`
	Item         jsoniter.RawMessage `json:"item"`
}

type txtCompletionMessage struct {
	Type         int                 `json:"type"`
	InvocationID string              `json:"invocationId"`
	Result       jsoniter.RawMessage `json:"result,omitempty"`
	Error        string              `json:"error,omitempty"`
}

type txtError struct {
	raw string
	err error
}

func (j *txtError) Error() string {
	return fmt.Sprintf("%v (source: %v)", j.err, j.raw)
}

// UnmarshalArgument unmarshals a jsoniter.RawMessage depending on the specified value type into value
func (j *txtHubProtocol) UnmarshalArgument(src interface{}, dst interface{}) error {
	rawSrc, ok := src.(jsoniter.RawMessage)
	if !ok {
		return fmt.Errorf("invalid source %#v for UnmarshalArgument", src)
	}
	if err := jsoniter.Unmarshal(rawSrc, dst); err != nil {
		return &txtError{string(rawSrc), err}
	}
	_ = j.dbg.Log(evt, "UnmarshalArgument",
		"argument", string(rawSrc),
		"value", fmt.Sprintf("%v", reflect.ValueOf(dst).Elem()))
	return nil
}

// ParseMessages reads all messages from the reader and puts the remaining bytes into remainBuf
func (j *txtHubProtocol) ParseMessages(reader io.Reader, remainBuf *bytes.Buffer) (messages []interface{}, err error) {
	frames, err := j.readTxtFrames(reader, remainBuf)
	if err != nil {
		return nil, err
	}
	message := hubMessage{}
	messages = make([]interface{}, 0)
	for _, frame := range frames {
		for i, v := range frame {
			frame[i] = 0xff - v
		}
		frame, _ = msgpack.NewDecoder(bytes.NewReader(frame)).DecodeBytes()
		err = jsoniter.Unmarshal(frame, &message)
		_ = j.dbg.Log(evt, "read", msg, string(frame))
		if err != nil {
			return nil, &txtError{string(frame), err}
		}
		typedMessage, err := j.parseMessage(message.Type, frame)
		if err != nil {
			return nil, err
		}
		// No specific type (aka Ping), use hubMessage
		if typedMessage == nil {
			typedMessage = message
		}
		messages = append(messages, typedMessage)
	}
	return messages, nil
}

func (j *txtHubProtocol) parseMessage(messageType int, text []byte) (message interface{}, err error) {
	switch messageType {
	case 1, 4:
		jsonInvocation := txtInvocationMessage{}
		if err = jsoniter.Unmarshal(text, &jsonInvocation); err != nil {
			err = &txtError{string(text), err}
		}
		arguments := make([]interface{}, len(jsonInvocation.Arguments))
		for i, a := range jsonInvocation.Arguments {
			arguments[i] = a
		}
		return invocationMessage{
			Type:         jsonInvocation.Type,
			Target:       jsonInvocation.Target,
			InvocationID: jsonInvocation.InvocationID,
			Arguments:    arguments,
			StreamIds:    jsonInvocation.StreamIds,
		}, err
	case 2:
		jsonStreamItem := txtStreamItemMessage{}
		if err = jsoniter.Unmarshal(text, &jsonStreamItem); err != nil {
			err = &txtError{string(text), err}
		}
		return streamItemMessage{
			Type:         jsonStreamItem.Type,
			InvocationID: jsonStreamItem.InvocationID,
			Item:         jsonStreamItem.Item,
		}, err
	case 3:
		jsonCompletion := txtCompletionMessage{}
		if err = jsoniter.Unmarshal(text, &jsonCompletion); err != nil {
			err = &txtError{string(text), err}
		}
		completion := completionMessage{
			Type:         jsonCompletion.Type,
			InvocationID: jsonCompletion.InvocationID,
			Error:        jsonCompletion.Error,
		}
		// Only assign Result when non nil. setting interface{} Result to (jsoniter.RawMessage)(nil)
		// will produce a value which can not compared to nil even if it is pointing towards nil!
		// See https://www.calhoun.io/when-nil-isnt-equal-to-nil/ for explanation
		if jsonCompletion.Result != nil {
			completion.Result = jsonCompletion.Result
		}
		return completion, err
	case 5:
		invocation := cancelInvocationMessage{}
		if err = jsoniter.Unmarshal(text, &invocation); err != nil {
			err = &txtError{string(text), err}
		}
		return invocation, err
	case 7:
		cm := closeMessage{}
		if err = jsoniter.Unmarshal(text, &cm); err != nil {
			err = &txtError{string(text), err}
		}
		return cm, err
	default:
		return nil, nil
	}
}
func (m *txtHubProtocol) readTxtFrames(reader io.Reader, remainBuf *bytes.Buffer) ([][]byte, error) {
	frames := make([][]byte, 0)
	for {
		// Try to get the frame length
		frameLenBuf := make([]byte, binary.MaxVarintLen32)
		n1, err := remainBuf.Read(frameLenBuf)
		if err != nil && !errors.Is(err, io.EOF) {
			// Some weird other error
			return nil, err
		}
		n2, err := reader.Read(frameLenBuf[n1:])
		if err != nil && !errors.Is(err, io.EOF) {
			// Some weird other error
			return nil, err
		}
		frameLen, lenLen := binary.Uvarint(frameLenBuf[:n1+n2])
		if lenLen == 0 {
			// reader could not supply enough bytes to decode the Uvarint
			// Store the already read bytes in the remainBuf for next iteration
			_, _ = remainBuf.Write(frameLenBuf[:n1+n2])
			return frames, nil
		}
		if lenLen < 0 {
			return nil, fmt.Errorf("messagepack frame length to large")
		}
		// Still wondering why this happens, but it happens!
		if frameLen == 0 {
			// Store the overread bytes for the next iteration
			_, _ = remainBuf.Write(frameLenBuf[lenLen:])
			continue
		}
		// Try getting data until at least one frame is available
		readBuf := make([]byte, frameLen)
		frameBuf := &bytes.Buffer{}
		// Did we read too many bytes when detecting the frameLen?
		_, _ = frameBuf.Write(frameLenBuf[lenLen:])
		// Read the rest of the bytes from the last iteration
		_, _ = frameBuf.ReadFrom(remainBuf)
		for {
			n, err := reader.Read(readBuf)
			if errors.Is(err, io.EOF) {
				// Less than frameLen. Let the caller parse the already read frames and come here again later
				_, _ = remainBuf.ReadFrom(frameBuf)
				return frames, nil
			}
			if err != nil {
				return nil, err
			}
			_, _ = frameBuf.Write(readBuf[:n])
			if frameBuf.Len() == int(frameLen) {
				// Frame completely read. Return it to the caller
				frames = append(frames, frameBuf.Next(int(frameLen)))
				return frames, nil
			}
			if frameBuf.Len() > int(frameLen) {
				// More than frameLen. Append the current frame to the result and start reading the next frame
				frames = append(frames, frameBuf.Next(int(frameLen)))
				_, _ = remainBuf.ReadFrom(frameBuf)
				break
			}
		}
	}
}

// WriteMessage writes a message as JSON to the specified writer
func (j *txtHubProtocol) WriteMessage(message interface{}, writer io.Writer) error {
	buffer, err := jsoniter.Marshal(message)
	if err != nil {
		return err
	}

	_ = j.dbg.Log(evt, "write", msg, string(buffer))
	buf := &bytes.Buffer{}
	msgpack.NewEncoder(buf).Encode(string(buffer))

	// Build frame with length information
	frameBuf := &bytes.Buffer{}
	lenBuf := make([]byte, binary.MaxVarintLen32)
	lenLen := binary.PutUvarint(lenBuf, uint64(buf.Len()))
	if _, err := frameBuf.Write(lenBuf[:lenLen]); err != nil {
		return err
	}
	for _, v := range buf.Bytes() {
		frameBuf.WriteByte(0xff - v)
	}
	_, err = frameBuf.WriteTo(writer)
	return err
}

func (j *txtHubProtocol) transferMode() TransferMode {
	return BinaryTransferMode
}

func (j *txtHubProtocol) setDebugLogger(dbg StructuredLogger) {
	j.dbg = log.WithPrefix(dbg, "ts", log.DefaultTimestampUTC, "protocol", "Txt")
}
