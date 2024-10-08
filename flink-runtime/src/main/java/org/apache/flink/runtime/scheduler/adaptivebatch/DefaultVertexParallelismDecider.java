/*
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package org.apache.flink.runtime.scheduler.adaptivebatch;

import org.apache.flink.annotation.VisibleForTesting;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.configuration.JobManagerOptions;
import org.apache.flink.configuration.MemorySize;
import org.apache.flink.runtime.jobgraph.JobVertexID;
import org.apache.flink.util.MathUtils;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.List;

import static org.apache.flink.util.Preconditions.checkArgument;
import static org.apache.flink.util.Preconditions.checkNotNull;
import static org.apache.flink.util.Preconditions.checkState;

/**
 * Default implementation of {@link VertexParallelismDecider}. Currently, in order to make the
 * number of subpartitions evenly consumed by downstream tasks, we will normalize the decided
 * parallelism to a power of 2.
 */
public class DefaultVertexParallelismDecider implements VertexParallelismDecider {

    private static final Logger LOG =
            LoggerFactory.getLogger(DefaultVertexParallelismDecider.class);

    /**
     * The cap ratio of broadcast bytes to data volume per task. The cap ratio is 0.5 currently
     * because we usually expect the broadcast dataset to be smaller than non-broadcast. We can make
     * it configurable later if we see users requesting for it.
     */
    private static final double CAP_RATIO_OF_BROADCAST = 0.5;

    private final int globalMaxParallelism;
    private final int globalMinParallelism;
    private final long dataVolumePerTask;
    private final int globalDefaultSourceParallelism;

    private DefaultVertexParallelismDecider(
            int globalMaxParallelism,
            int globalMinParallelism,
            MemorySize dataVolumePerTask,
            int globalDefaultSourceParallelism) {

        checkArgument(globalMinParallelism > 0, "The minimum parallelism must be larger than 0.");
        checkArgument(
                globalMaxParallelism >= globalMinParallelism,
                "Maximum parallelism should be greater than or equal to the minimum parallelism.");
        checkArgument(
                globalDefaultSourceParallelism > 0,
                "The default source parallelism must be larger than 0.");
        checkNotNull(dataVolumePerTask);

        this.globalMaxParallelism = globalMaxParallelism;
        this.globalMinParallelism = globalMinParallelism;
        this.dataVolumePerTask = dataVolumePerTask.getBytes();
        this.globalDefaultSourceParallelism = globalDefaultSourceParallelism;
    }

    @Override
    public int decideParallelismForVertex(
            JobVertexID jobVertexId,
            List<BlockingResultInfo> consumedResults,
            int vertexMaxParallelism) {

        if (consumedResults.isEmpty()) {
            // source job vertex
            return computeSourceParallelism(jobVertexId, vertexMaxParallelism);
        } else {
            int minParallelism = globalMinParallelism;
            int maxParallelism = globalMaxParallelism;
            vertexMaxParallelism = getNormalizedMaxParallelism(vertexMaxParallelism);

            if (vertexMaxParallelism < minParallelism) {
                LOG.info(
                        "The vertex maximum parallelism {} is smaller than the global minimum parallelism {}. "
                                + "Use {} as the lower bound to decide parallelism of job vertex {}.",
                        vertexMaxParallelism,
                        minParallelism,
                        vertexMaxParallelism,
                        jobVertexId);
                minParallelism = vertexMaxParallelism;
            }
            if (vertexMaxParallelism < maxParallelism) {
                LOG.info(
                        "The vertex maximum parallelism {} is smaller than the global maximum parallelism {}. "
                                + "Use {} as the upper bound to decide parallelism of job vertex {}.",
                        vertexMaxParallelism,
                        maxParallelism,
                        vertexMaxParallelism,
                        jobVertexId);
                maxParallelism = vertexMaxParallelism;
            }
            checkState(maxParallelism >= minParallelism);
            return computeParallelism(jobVertexId, consumedResults, maxParallelism, minParallelism);
        }
    }

    private int computeSourceParallelism(JobVertexID jobVertexId, int maxParallelism) {
        if (globalDefaultSourceParallelism > maxParallelism) {
            LOG.info(
                    "The global default source parallelism {} is larger than the maximum parallelism {}. "
                            + "Use {} as the parallelism of source job vertex {}.",
                    globalDefaultSourceParallelism,
                    maxParallelism,
                    maxParallelism,
                    jobVertexId);
            return maxParallelism;
        } else {
            return globalDefaultSourceParallelism;
        }
    }

    private int computeParallelism(
            JobVertexID jobVertexId,
            List<BlockingResultInfo> consumedResults,
            int maxParallelism,
            int minParallelism) {

        long broadcastBytes =
                consumedResults.stream()
                        .filter(BlockingResultInfo::isBroadcast)
                        .mapToLong(
                                consumedResult ->
                                        consumedResult.getBlockingPartitionSizes().stream()
                                                .reduce(0L, Long::sum))
                        .sum();

        long nonBroadcastBytes =
                consumedResults.stream()
                        .filter(consumedResult -> !consumedResult.isBroadcast())
                        .mapToLong(
                                consumedResult ->
                                        consumedResult.getBlockingPartitionSizes().stream()
                                                .reduce(0L, Long::sum))
                        .sum();

        long expectedMaxBroadcastBytes =
                (long) Math.ceil((dataVolumePerTask * CAP_RATIO_OF_BROADCAST));

        if (broadcastBytes > expectedMaxBroadcastBytes) {
            LOG.info(
                    "The size of broadcast data {} is larger than the expected maximum value {} ('{}' * {})."
                            + " Use {} as the size of broadcast data to decide the parallelism of job vertex {}.",
                    new MemorySize(broadcastBytes),
                    new MemorySize(expectedMaxBroadcastBytes),
                    JobManagerOptions.ADAPTIVE_BATCH_SCHEDULER_AVG_DATA_VOLUME_PER_TASK.key(),
                    CAP_RATIO_OF_BROADCAST,
                    new MemorySize(expectedMaxBroadcastBytes),
                    jobVertexId);

            broadcastBytes = expectedMaxBroadcastBytes;
        }

        int initialParallelism =
                (int) Math.ceil((double) nonBroadcastBytes / (dataVolumePerTask - broadcastBytes));
        int parallelism = normalizeParallelism(initialParallelism);

        LOG.debug(
                "The size of broadcast data is {}, the size of non-broadcast data is {}, "
                        + "the initially decided parallelism of job vertex {} is {}, after normalization is {}",
                new MemorySize(broadcastBytes),
                new MemorySize(nonBroadcastBytes),
                jobVertexId,
                initialParallelism,
                parallelism);

        if (parallelism < minParallelism) {
            LOG.info(
                    "The initially normalized parallelism {} is smaller than the normalized minimum parallelism {}. "
                            + "Use {} as the finally decided parallelism of job vertex {}.",
                    parallelism,
                    minParallelism,
                    minParallelism,
                    jobVertexId);
            parallelism = minParallelism;
        } else if (parallelism > maxParallelism) {
            LOG.info(
                    "The initially normalized parallelism {} is larger than the normalized maximum parallelism {}. "
                            + "Use {} as the finally decided parallelism of job vertex {}.",
                    parallelism,
                    maxParallelism,
                    maxParallelism,
                    jobVertexId);
            parallelism = maxParallelism;
        }

        return parallelism;
    }

    @VisibleForTesting
    int getGlobalMaxParallelism() {
        return globalMaxParallelism;
    }

    @VisibleForTesting
    int getGlobalMinParallelism() {
        return globalMinParallelism;
    }

    static DefaultVertexParallelismDecider from(Configuration configuration) {
        int maxParallelism = getNormalizedMaxParallelism(configuration);
        int minParallelism = getNormalizedMinParallelism(configuration);
        checkState(
                maxParallelism >= minParallelism,
                String.format(
                        "Invalid configuration: '%s' should be greater than or equal to '%s' and the range must contain at least one power of 2.",
                        JobManagerOptions.ADAPTIVE_BATCH_SCHEDULER_MAX_PARALLELISM.key(),
                        JobManagerOptions.ADAPTIVE_BATCH_SCHEDULER_MIN_PARALLELISM.key()));

        return new DefaultVertexParallelismDecider(
                maxParallelism,
                minParallelism,
                configuration.get(
                        JobManagerOptions.ADAPTIVE_BATCH_SCHEDULER_AVG_DATA_VOLUME_PER_TASK),
                configuration.get(
                        JobManagerOptions.ADAPTIVE_BATCH_SCHEDULER_DEFAULT_SOURCE_PARALLELISM));
    }

    private static int getNormalizedMaxParallelism(int maxParallelism) {
        return MathUtils.roundDownToPowerOf2(maxParallelism);
    }

    static int getNormalizedMaxParallelism(Configuration configuration) {
        return getNormalizedMaxParallelism(
                configuration.getInteger(
                        JobManagerOptions.ADAPTIVE_BATCH_SCHEDULER_MAX_PARALLELISM));
    }

    static int getNormalizedMinParallelism(Configuration configuration) {
        return MathUtils.roundUpToPowerOfTwo(
                configuration.getInteger(
                        JobManagerOptions.ADAPTIVE_BATCH_SCHEDULER_MIN_PARALLELISM));
    }

    static int normalizeParallelism(int parallelism) {
        int down = MathUtils.roundDownToPowerOf2(parallelism);
        int up = MathUtils.roundUpToPowerOfTwo(parallelism);
        return parallelism < (up + down) / 2 ? down : up;
    }
}
