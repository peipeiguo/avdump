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
	ret := decodeCtx.AvcodecSendPacket(packet)
	if ret < 0 {
		log.Printf("Error while sending a packet to the decoder: %s\n", avutil.ErrorFromCode(ret))
	}

	for ret >= 0 {
		ret = decodeCtx.AvcodecReceiveFrame((*avcodec.Frame)(unsafe.Pointer(frame)))
		if ret == avutil.AvErrorEAGAIN || ret == avutil.AvErrorEOF {
			break
		} else if ret < 0 {
			log.Printf("Error while receiving a frame from the decoder: %s\n", avutil.ErrorFromCode(ret))
			return
		}

		// process image
		log.Printf("frame: media_type=video, stream_index=%d, key_frame=%d, pts=%d (diff=%d), pkt_dts=%d (diff=%d), pkt_duration=%d, pkt_pts=%d (diff=%d), frame_rate=%d, pict_type=%s, coded_pict_number=%d, display_pict_number=%d",
			packet.StreamIndex(), frame.KeyFrame(), frame.Pts(), frame.Pts()-prevVFrame.pts,
			frame.PktDts(), frame.PktDts()-prevVFrame.pktDts, packet.Duration(),
			packet.Pts(), packet.Pts()-prevVFrame.pktPts, decodeCtx.GetFrameRateInt(),
			avutil.AvGetPictureTypeChar(avutil.AvPictureType(frame.PictureType())), frame.CodedPictureNumber(), frame.DisplayPictureNumber())
		prevVFrame.pts = frame.Pts()
		prevVFrame.pktDts = frame.PktDts()
		prevVFrame.pktPts = packet.Pts()
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
			log.Printf("Error while receiving a frame from the decoder: %s\n", avutil.ErrorFromCode(response))
			return
		}

		// process sample
		log.Printf("frame: media_type=audio, stream_index=%d, key_frame=%d, pts=%d (diff=%d), pkt_dts=%d (diff=%d), pkt_duration=%d, pkt_pts=%d (diff=%d)",
			packet.StreamIndex(), frame.KeyFrame(), frame.Pts(), frame.Pts()-prevAFrame.pts,
			frame.PktDts(), frame.PktDts()-prevVFrame.pktDts, packet.Duration(),
			packet.Pts(), packet.Pts()-prevVFrame.pktPts)
		prevAFrame.pts = frame.Pts()
		prevAFrame.pktDts = frame.PktDts()
		prevAFrame.pktPts = packet.Pts()
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
			log.Printf("stream: media_type=video, stream_index=%d, timebase=%d/%d, codec_timebase=%d/%d", i,
				stream.TimeBase().Num(), stream.TimeBase().Den(),
				vDecCtx.AvCodecGetTimebase().Num(), vDecCtx.AvCodecGetTimebase().Den())
		} else if mediaType == avformat.AVMEDIA_TYPE_AUDIO {
			aDecCtx = openDecoder(formatCtx, stream)
			log.Printf("stream: media_type=audio, stream_index=%d, timebase=%d/%d, codec_timebase=%d/%d", i,
				stream.TimeBase().Num(), stream.TimeBase().Den(),
				aDecCtx.AvCodecGetTimebase().Num(), aDecCtx.AvCodecGetTimebase().Den())
		} else {
			log.Printf("stream: media_type=%v, stream_index=%d", mediaType, i)
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
			log.Printf("frame: media_type=%v, stream_index=%d, pkt_pts=%d, pkt_dts=%d, pkt_duration=%d", mediaType,
				packet.StreamIndex(), packet.Pts(), packet.Dts(), packet.Duration())
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
	fmt.Fprintf(os.Stderr, "Usage: avdump [-s stream_index] -i input_url\nOptions:\n")
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

	if len(os.Args) < 3 {
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
