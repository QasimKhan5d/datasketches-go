/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package cpc

import "fmt"

type CpcCompressedState struct {
	CsvIsValid    bool
	WindowIsValid bool
	LgK           int
	SeedHash      int16
	FiCol         int
	MergeFlag     bool // compliment of HIP Flag
	NumCoupons    uint64

	Kxp         float64
	HipEstAccum float64

	NumCsv        int
	CsvStream     []int // may be longer than required
	CsvLengthInts int
	CwStream      []int // may be longer than required
	CwLengthInts  int
}

var (
	// This defines the preamble space required by each of the formats in units of 4-byte integers.
	preIntsDefs = []byte{2, 2, 4, 8, 4, 8, 6, 10}
)

func NewCpcCompressedState(lgK int, seedHash int16) *CpcCompressedState {
	return &CpcCompressedState{
		LgK:      lgK,
		SeedHash: seedHash,
		Kxp:      float64(int(1) << lgK),
	}
}

func (c *CpcCompressedState) getRequiredSerializedBytes() int {
	preInts := getDefinedPreInts(c.getFormat())
	return 4 * (preInts + c.CsvLengthInts + c.CwLengthInts)
}

func (c *CpcCompressedState) getWindowOffset() int {
	return determineCorrectOffset(c.LgK, c.NumCoupons)
}

func (c *CpcCompressedState) getFormat() CpcFormat {
	ordinal := 0
	if c.CwLengthInts > 0 {
		ordinal |= 4
	}
	if c.NumCsv > 0 {
		ordinal |= 2
	}
	if c.MergeFlag {
		ordinal |= 1
	}
	return CpcFormat(ordinal)
}

func (c *CpcCompressedState) uncompress(seed uint64) (*CpcSketch, error) {
	//ThetaUtil.checkSeedHashes(ThetaUtil.computeSeedHash(seed), c.SeedHash)
	sketch, err := NewCpcSketch(c.LgK, seed)
	if err != nil {
		return nil, err
	}
	sketch.numCoupons = c.NumCoupons
	sketch.windowOffset = c.getWindowOffset()
	sketch.fiCol = c.FiCol
	sketch.mergeFlag = c.MergeFlag
	sketch.kxp = c.Kxp
	sketch.hipEstAccum = c.HipEstAccum
	sketch.slidingWindow = nil
	sketch.pairTable = nil
	//uncompress(c, sketch)
	return sketch, err
}

/*
  //also used in test
  static CpcSketch uncompress(final CompressedState source, final long seed) {
    ThetaUtil.checkSeedHashes(ThetaUtil.computeSeedHash(seed), source.seedHash);
    final CpcSketch sketch = new CpcSketch(source.lgK, seed);
    sketch.numCoupons = source.numCoupons;
    sketch.windowOffset = source.getWindowOffset();
    sketch.fiCol = source.fiCol;
    sketch.mergeFlag = source.mergeFlag;
    sketch.kxp = source.kxp;
    sketch.hipEstAccum = source.hipEstAccum;
    sketch.slidingWindow = null;
    sketch.pairTable = null;
    CpcCompression.uncompress(source, sketch);
    return sketch;
  }
*/

func importFromMemory(bytes []byte) (*CpcCompressedState, error) {
	if err := checkLoPreamble(bytes); err != nil {
		return nil, err
	}
	if !isCompressed(bytes) {
		return nil, fmt.Errorf("not compressed")
	}
	lgK := getLgK(bytes)
	seedHash := getSeedHash(bytes)
	state := NewCpcCompressedState(lgK, seedHash)
	fmtOrd := getFormatOrdinal(bytes)
	format := CpcFormat(fmtOrd)
	state.MergeFlag = (fmtOrd & 1) == 0
	state.CsvIsValid = (fmtOrd & 2) > 0
	state.WindowIsValid = (fmtOrd & 4) > 0

	switch format {
	case CpcformatEmptyMerged, CpcFormatEmptyHip:
		if err := checkCapacity(len(bytes), 8); err != nil {
			return nil, err
		}
	case CpcFormatSparseHybridMerged:
		state.NumCoupons = getNumCoupons(bytes)
		state.NumCsv = int(state.NumCoupons)
		state.CsvLengthInts = getSvLengthInts(bytes)
		if err := checkCapacity(len(bytes), state.getRequiredSerializedBytes()); err != nil {
			return nil, err
		}
		state.CsvStream = getSvStream(bytes)
	case CpcFormatSparceHybridHip:
		state.NumCoupons = getNumCoupons(bytes)
		state.NumCsv = int(state.NumCoupons)
		state.CsvLengthInts = getSvLengthInts(bytes)
		state.Kxp = getKxP(bytes)
		state.HipEstAccum = getHipAccum(bytes)
		if err := checkCapacity(len(bytes), state.getRequiredSerializedBytes()); err != nil {
			return nil, err
		}
		state.CsvStream = getSvStream(bytes)
	default:
		panic("not implemented")
	}
	return state, nil
}

/*
static CompressedState importFromMemory(final Memory mem) {
    checkLoPreamble(mem);
    rtAssert(isCompressed(mem));
    final int lgK = getLgK(mem);
    final short seedHash = getSeedHash(mem);
    final CompressedState state = new CompressedState(lgK, seedHash);
    final int fmtOrd = getFormatOrdinal(mem);
    final Format format = Format.ordinalToFormat(fmtOrd);
    state.mergeFlag = !((fmtOrd & 1) > 0); //merge flag is complement of HIP
    state.csvIsValid = (fmtOrd & 2) > 0;
    state.windowIsValid = (fmtOrd & 4) > 0;

    switch (format) {
      case EMPTY_MERGED :
      case EMPTY_HIP : {
        checkCapacity(mem.getCapacity(), 8L);
        break;
      }
      case SPARSE_HYBRID_MERGED : {
        //state.fiCol = getFiCol(mem);
        state.numCoupons = getNumCoupons(mem);
        state.numCsv = (int) state.numCoupons; //only true for sparse_hybrid
        state.csvLengthInts = getSvLengthInts(mem);
        //state.cwLength = getCwLength(mem);
        //state.kxp = getKxP(mem);
        //state.hipEstAccum = getHipAccum(mem);
        checkCapacity(mem.getCapacity(), state.getRequiredSerializedBytes());
        //state.cwStream = getCwStream(mem);
        state.csvStream = getSvStream(mem);
        break;
      }
      case SPARSE_HYBRID_HIP : {
        //state.fiCol = getFiCol(mem);
        state.numCoupons = getNumCoupons(mem);
        state.numCsv = (int) state.numCoupons; //only true for sparse_hybrid
        state.csvLengthInts = getSvLengthInts(mem);
        //state.cwLength = getCwLength(mem);
        state.kxp = getKxP(mem);
        state.hipEstAccum = getHipAccum(mem);
        checkCapacity(mem.getCapacity(), state.getRequiredSerializedBytes());
        //state.cwStream = getCwStream(mem);
        state.csvStream = getSvStream(mem);
        break;
      }
      case PINNED_SLIDING_MERGED_NOSV : {
        state.fiCol = getFiCol(mem);
        state.numCoupons = getNumCoupons(mem);
        //state.numCsv = getNumCsv(mem);
        //state.csvLength = getCsvLength(mem);
        state.cwLengthInts = getWLengthInts(mem);
        //state.kxp = getKxP(mem);
        //state.hipEstAccum = getHipAccum(mem);
        checkCapacity(mem.getCapacity(), state.getRequiredSerializedBytes());
        state.cwStream = getWStream(mem);
        //state.csvStream = getCsvStream(mem);
        break;
      }
      case PINNED_SLIDING_HIP_NOSV : {
        state.fiCol = getFiCol(mem);
        state.numCoupons = getNumCoupons(mem);
        //state.numCsv = getNumCsv(mem);
        //state.csvLength = getCsvLength(mem);
        state.cwLengthInts = getWLengthInts(mem);
        state.kxp = getKxP(mem);
        state.hipEstAccum = getHipAccum(mem);
        checkCapacity(mem.getCapacity(), state.getRequiredSerializedBytes());
        state.cwStream = getWStream(mem);
        //state.csvStream = getCsvStream(mem);
        break;
      }
      case PINNED_SLIDING_MERGED : {
        state.fiCol = getFiCol(mem);
        state.numCoupons = getNumCoupons(mem);
        state.numCsv = getNumSv(mem);
        state.csvLengthInts = getSvLengthInts(mem);
        state.cwLengthInts = getWLengthInts(mem);
        //state.kxp = getKxP(mem);
        //state.hipEstAccum = getHipAccum(mem);
        checkCapacity(mem.getCapacity(), state.getRequiredSerializedBytes());
        state.cwStream = getWStream(mem);
        state.csvStream = getSvStream(mem);
        break;
      }
      case PINNED_SLIDING_HIP : {
        state.fiCol = getFiCol(mem);
        state.numCoupons = getNumCoupons(mem);
        state.numCsv = getNumSv(mem);
        state.csvLengthInts = getSvLengthInts(mem);
        state.cwLengthInts = getWLengthInts(mem);
        state.kxp = getKxP(mem);
        state.hipEstAccum = getHipAccum(mem);
        checkCapacity(mem.getCapacity(), state.getRequiredSerializedBytes());
        state.cwStream = getWStream(mem);
        state.csvStream = getSvStream(mem);
        break;
      }
    }
    checkCapacity(mem.getCapacity(),
        4L * (getPreInts(mem) + state.csvLengthInts + state.cwLengthInts));
    return state;
  }
*/

func getDefinedPreInts(format CpcFormat) int {
	return int(preIntsDefs[format])
}
