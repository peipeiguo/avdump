package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"unsafe"

	"github.com/leokinglong/goav/avcodec"
	"github.com/leokinglong/goav/avformat"
	"github.com/leokinglong/goav/avutil"
)

type frameInfo struct {
	pktDts int64
	pktPts int64
	pts    int64
}

var (
	prevAFrame frameInfo
	prevVFrame frameInfo
)

/**
 * Open decoder
 */
func openDecoder(formatCtx *avformat.Context, stream *avformat.Stream) *avcodec.Context {
	// Find the decoder for the video stream
	decoder := avcodec.AvcodecFindDecoder(stream.CodecParameters().AvCodecGetId())
	if decoder == nil {
		log.Printf("Unsupported codec")
		return nil
	}

	// Copy context
	decodeCtx := decoder.AvcodecAllocContext3()
	avcodec.AvcodecParametersToContext(decodeCtx, stream.CodecParameters())
	if decodeCtx.CodecType() == avformat.AVMEDIA_TYPE_VIDEO {
		//frameRate := decodeCtx.AvGuessFrameRate(stream, nil)
		decodeCtx.SetThreadCount(8)
		decodeCtx.SetThreadType(1)
	}

	// Open codec
	if decodeCtx.AvcodecOpen2(decoder, nil) < 0 {
		log.Printf("Can not open codec")
		return nil
	}

	return decodeCtx
}

/**
 * Dump video stream
 */
func dumpVideoFrame(decodeCtx *avcodec.Context, packet *avcodec.Packet) {
	// decode video frame
	frame := avutil.AvFrameAlloc()
	response := decodeCtx.AvcodecSendPacket(packet)
	if response < 0 {
		log.Printf("Error while sending a packet to the decoder: %s\n", avutil.ErrorFromCode(response))
	}

	for response >= 0 {
		response = decodeCtx.AvcodecReceiveFrame((*avcodec.Frame)(unsafe.Pointer(frame)))
		if response == avutil.AvErrorEAGAIN || response == avutil.AvErrorEOF {
			break
		} else if response < 0 {
			log.Print("Error while receiving a frame from the decoder: %s\n", avutil.ErrorFromCode(response))
			return
		}

		// process image
		log.Printf("Stream %d: Video: pkt pts %d (diff %d), frame pts %d (diff %d), pkt dts %d (diff %d), pkt duration %d, frameRate %d ",
			packet.StreamIndex(), packet.Pts(), packet.Pts()-prevVFrame.pktPts, frame.Pts(), frame.Pts()-prevVFrame.pts,
			packet.Dts(), packet.Dts()-prevVFrame.pktDts, packet.Duration(), decodeCtx.GetFrameRateInt())
		prevVFrame.pktDts = packet.Dts()
		prevVFrame.pktPts = packet.Pts()
		prevVFrame.pts = frame.Pts()
	}

	// Free the YUV frame
	avutil.AvFrameFree(frame)
}

/**
 * Dump audio stream
 */
func dumpAudioFrame(decodeCtx *avcodec.Context, packet *avcodec.Packet) {
	// decode audio frame
	frame := avutil.AvFrameAlloc()
	response := decodeCtx.AvcodecSendPacket(packet)
	if response < 0 {
		log.Printf("Error while sending a packet to the decoder: %s\n", avutil.ErrorFromCode(response))
	}

	for response >= 0 {
		response = decodeCtx.AvcodecReceiveFrame((*avcodec.Frame)(unsafe.Pointer(frame)))
		if response == avutil.AvErrorEAGAIN || response == avutil.AvErrorEOF {
			break
		} else if response < 0 {
			log.Print("Error while receiving a frame from the decoder: %s\n", avutil.ErrorFromCode(response))
			return
		}

		// process sample
		log.Printf("Stream %d: Audio: pkt pts %d (diff %d), frame pts %d (diff %d), pkt dts %d (diff %d), pkt duration %d",
			packet.StreamIndex(), packet.Pts(), packet.Pts()-prevAFrame.pktPts, frame.Pts(), frame.Pts()-prevAFrame.pts,
			packet.Dts(), packet.Dts()-prevAFrame.pktDts, packet.Duration())
		prevAFrame.pktDts = packet.Dts()
		prevAFrame.pktPts = packet.Pts()
		prevAFrame.pts = frame.Pts()
	}

	// Free the YUV frame
	avutil.AvFrameFree(frame)
}

/**
 * Dump stream
 */
func dumpStream(formatCtx *avformat.Context, sIndex int) {
	log.Printf("Dump stream")

	// Open the codecs for video and audio
	var vDecCtx *avcodec.Context
	var aDecCtx *avcodec.Context
	for i := 0; i < int(formatCtx.NbStreams()); i++ {
		stream := formatCtx.Streams()[i]
		mediaType := stream.CodecParameters().AvCodecGetType()
		if mediaType == avformat.AVMEDIA_TYPE_VIDEO {
			vDecCtx = openDecoder(formatCtx, stream)
			log.Printf("Stream %d: Video: timebase=%d/%d, codec timebase: %d/%d", i,
				stream.TimeBase().Num(), stream.TimeBase().Den(),
				vDecCtx.AvCodecGetTimebase().Num(), vDecCtx.AvCodecGetTimebase().Den())
		} else if mediaType == avformat.AVMEDIA_TYPE_AUDIO {
			aDecCtx = openDecoder(formatCtx, stream)
			log.Printf("Stream %d: Audio: timebase=%d/%d, codec timebase: %d/%d", i,
				stream.TimeBase().Num(), stream.TimeBase().Den(),
				aDecCtx.AvCodecGetTimebase().Num(), aDecCtx.AvCodecGetTimebase().Den())
		} else {
			log.Printf("Stream %d: Unsupported stream: %v", i, mediaType)
			continue
		}
	}

	packet := avcodec.AvPacketAlloc()
	for formatCtx.AvReadFrame(packet) >= 0 {
		if sIndex != -1 && sIndex != packet.StreamIndex() {
			continue
		}

		stream := formatCtx.Streams()[packet.StreamIndex()]
		mediaType := stream.CodecParameters().AvCodecGetType()
		if mediaType == avformat.AVMEDIA_TYPE_VIDEO {
			dumpVideoFrame(vDecCtx, packet)
		} else if mediaType == avformat.AVMEDIA_TYPE_AUDIO {
			dumpAudioFrame(aDecCtx, packet)
		} else {
			log.Printf("Stream %d: pts %d dts %d duration %d", packet.StreamIndex(), packet.Pts(), packet.Dts(), packet.Duration())
			continue
		}
	}

	// Free the packet that was allocated by av_read_frame
	packet.AvPacketFree()

	// Close the codecs
	vDecCtx.AvcodecClose()
	aDecCtx.AvcodecClose()
}

/**
 * Print usage
 */
func usage() {
	fmt.Fprintf(os.Stderr, "Usage: avdump [-s stream_index] input_file\nOptions:\n")
	flag.PrintDefaults()
}

func main() {
	// Define command line options
	var sIndex int
	var url string
	flag.IntVar(&sIndex, "s", -1, "Stream index to dump in input file, default is dump all streams")
	flag.StringVar(&url, "i", "", "Input URL, such as file path or URL")
	flag.Usage = usage
	flag.Parse()

	if len(os.Args) < 2 {
		flag.Usage()
		os.Exit(1)
	}

	// Register all formats and codecs
	// avformat.AvRegisterAll()
	// avcodec.AvcodecRegisterAll()

	// Open input stream and read the header.
	log.Printf("Open input stream and read the header: %s", url)
	formatCtx := avformat.AvformatAllocContext()
	var avDict *avutil.Dictionary
	avDict.AvDictSet("probesize", "100 *1024", 0)
	avDict.AvDictSet("max_analyze_duration", "2", 0)
	avDict.AvDictSet("rw_timeout", "10000000", 0)
	if avformat.AvformatOpenInput(&formatCtx, url, nil, &avDict) != 0 {
		log.Printf("Unable to open stream %s\n", url)
		os.Exit(1)
	}

	// Retrieve stream information
	log.Printf("Retrieve stream information")
	if formatCtx.AvformatFindStreamInfo(&avDict) < 0 {
		log.Printf("Couldn't find stream information")
		formatCtx.AvformatCloseInput()
		os.Exit(1)
	}

	// Dump information about stream onto standard error
	formatCtx.AvDumpFormat(0, url, 0)

	// Dump information about stream
	dumpStream(formatCtx, sIndex)

	// Close the video file
	log.Printf("Close the video file")
	formatCtx.AvformatCloseInput()
}
